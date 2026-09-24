package pricing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

var errEmptyCatalog = errors.New("official price catalog is empty")

// ParseOfficialMarkdown reads the flagship Standard and Fast tables from the
// OpenAI pricing document. Batch, Flex, and later specialized tables are ignored.
// A dash means that rate is unpublished. Both tables must be present.
func ParseOfficialMarkdown(markdown string) (map[string]ModelSchedule, error) {
	lines := strings.Split(markdown, "\n")
	label := ""
	rates := map[string]ModelSchedule{}
	sawStandard, sawFast := false, false
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			label = line
			continue
		}
		header := splitTableRow(line)
		if !isContextPriceHeader(header) {
			continue
		}
		kind := tableKind(label)
		if kind == "" || (kind == "standard" && sawStandard) || (kind == "fast" && sawFast) {
			index = skipTable(lines, index)
			continue
		}
		rows, next := readTable(lines, index+1)
		index = next
		if err := applyTable(rates, kind, header, rows); err != nil {
			return nil, err
		}
		if kind == "standard" {
			sawStandard = true
		} else {
			sawFast = true
		}
	}
	if !sawStandard || !sawFast || len(rates) < 3 {
		return nil, errors.New("official pricing document did not contain usable Standard and Fast tables")
	}
	return rates, nil
}

// OfficialScheduleID identifies a fetched document without treating the compiled seed as current.
func OfficialScheduleID(rates map[string]ModelSchedule, fetched time.Time) (string, error) {
	raw, err := json.Marshal(rates)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "openai-api-docs-" + fetched.UTC().Format("2006-01-02") + "-" + hex.EncodeToString(sum[:4]), nil
}

func tableKind(label string) string {
	label = strings.ToLower(label)
	switch {
	case strings.Contains(label, "batch"), strings.Contains(label, "flex"):
		return ""
	case strings.Contains(label, "fast"):
		return "fast"
	case strings.Contains(label, "standard"):
		return "standard"
	default:
		return ""
	}
}

func isContextPriceHeader(cells []string) bool {
	joined := strings.ToLower(strings.Join(cells, " | "))
	return strings.Contains(joined, "model") && strings.Contains(joined, "short context input") && strings.Contains(joined, "long context output")
}

func readTable(lines []string, start int) ([][]string, int) {
	rows := [][]string{}
	index := start
	for index < len(lines) {
		line := strings.TrimSpace(lines[index])
		if !strings.HasPrefix(line, "|") {
			break
		}
		cells := splitTableRow(line)
		if !isSeparatorRow(cells) {
			rows = append(rows, cells)
		}
		index++
	}
	return rows, index - 1
}

func skipTable(lines []string, start int) int {
	index := start + 1
	for index < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[index]), "|") {
		index++
	}
	return index - 1
}

func applyTable(rates map[string]ModelSchedule, kind string, header []string, rows [][]string) error {
	columns := map[string]int{}
	for index, cell := range header {
		columns[strings.ToLower(cell)] = index
	}
	needed := []string{"model", "short context input", "short context cached input", "short context cache writes", "short context output", "long context input", "long context cached input", "long context cache writes", "long context output"}
	for _, name := range needed {
		if _, ok := columns[name]; !ok {
			return errors.New("official pricing table is missing " + name)
		}
	}
	for _, row := range rows {
		rawModel := cell(row, columns["model"])
		model := normalizeOfficialModel(rawModel)
		if model == "" {
			continue
		}
		threshold := parseContextThreshold(rawModel)
		short, shortOK, err := ratesFrom(row, columns, "short context ")
		if err != nil {
			return err
		}
		long, longOK, err := ratesFrom(row, columns, "long context ")
		if err != nil {
			return err
		}
		if !shortOK {
			continue
		}
		schedule := rates[model]
		bands := PriceBands{Short: short}
		if longOK {
			bands.Long = &long
		}
		if kind == "fast" {
			schedule.Fast = &bands
		} else {
			schedule.Standard = bands
		}
		if threshold > 0 {
			schedule.ContextThresholdTokens = threshold
		} else if schedule.ContextThresholdTokens <= 0 {
			schedule.ContextThresholdTokens = LongContextTokens
		}
		rates[model] = schedule
	}
	return nil
}

func ratesFrom(row []string, columns map[string]int, prefix string) (TokenRates, bool, error) {
	input, inputOK, err := parsePriceCell(cell(row, columns[prefix+"input"]))
	if err != nil {
		return TokenRates{}, false, err
	}
	cached, cachedOK, err := parsePriceCell(cell(row, columns[prefix+"cached input"]))
	if err != nil {
		return TokenRates{}, false, err
	}
	write, writeOK, err := parsePriceCell(cell(row, columns[prefix+"cache writes"]))
	if err != nil {
		return TokenRates{}, false, err
	}
	output, outputOK, err := parsePriceCell(cell(row, columns[prefix+"output"]))
	if err != nil {
		return TokenRates{}, false, err
	}
	if !inputOK && !cachedOK && !writeOK && !outputOK {
		return TokenRates{}, false, nil
	}
	return TokenRates{Input: input, CachedInput: cached, CacheWrite: write, Output: output, HasCachedInput: cachedOK, HasCacheWrite: writeOK}, true, nil
}

func parsePriceCell(cell string) (float64, bool, error) {
	cell = strings.TrimSpace(cell)
	if cell == "" || cell == "-" || cell == "—" {
		return 0, false, nil
	}
	cell = strings.TrimPrefix(cell, "$")
	cell = strings.ReplaceAll(cell, ",", "")
	value, err := strconv.ParseFloat(cell, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false, errors.New("invalid official price cell " + cell)
	}
	return value, true, nil
}

func parseContextThreshold(cell string) int64 {
	lower := strings.ToLower(cell)
	idx := strings.Index(lower, "<")
	if idx >= 0 {
		sub := lower[idx+1:]
		if mIdx := strings.Index(sub, "m"); mIdx >= 0 && (strings.Index(sub, "k") < 0 || mIdx < strings.Index(sub, "k")) {
			numStr := strings.TrimSpace(sub[:mIdx])
			if n, err := strconv.ParseInt(numStr, 10, 64); err == nil && n > 0 {
				return n * 1_000_000
			}
		}
		if kIdx := strings.Index(sub, "k"); kIdx >= 0 {
			numStr := strings.TrimSpace(sub[:kIdx])
			if n, err := strconv.ParseInt(numStr, 10, 64); err == nil && n > 0 {
				return n * 1_000
			}
		}
	}
	return 0
}

func normalizeOfficialModel(cell string) string {
	cell = strings.ToLower(strings.TrimSpace(cell))
	if index := strings.IndexAny(cell, " ("); index >= 0 {
		cell = cell[:index]
	}
	if cell == "" || strings.Contains(cell, "|") {
		return ""
	}
	return cell
}

func cell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

func splitTableRow(line string) []string {
	parts := strings.Split(line, "|")
	if len(parts) > 0 && strings.TrimSpace(parts[0]) == "" {
		parts = parts[1:]
	}
	if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return true
}
