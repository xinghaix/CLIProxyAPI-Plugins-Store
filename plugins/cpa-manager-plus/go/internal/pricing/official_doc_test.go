package pricing

import (
	"strings"
	"testing"
	"time"
)

func officialFixture() string {
	return strings.Join([]string{
		"### Batch pricing data",
		"",
		"| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
		"| gpt-ignore | $99.00 | $99.00 | $99.00 | $99.00 | $99.00 | $99.00 | $99.00 | $99.00 |",
		"",
		"### Standard pricing data",
		"",
		"| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
		"| gpt-5.4 (<272K context length) | $2.50 | $0.25 | - | $15.00 | $5.00 | $0.50 | - | $22.50 |",
		"| gpt-5.5 (<272K context length) | $5.00 | $0.50 | - | $30.00 | $10.00 | $1.00 | - | $45.00 |",
		"| gpt-5.6-sol | $4.00 | $0.40 | $5.00 | $20.00 | $8.00 | $0.80 | $10.00 | $30.00 |",
		"| gpt-6-sol | $2.00 | $0.20 | $2.50 | $10.00 | $4.00 | $0.40 | $5.00 | $15.00 |",
		"",
		"### Fast pricing data",
		"",
		"| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
		"| gpt-5.4 (<272K context length) | $5.00 | $0.50 | - | $30.00 | - | - | - | - |",
		"| gpt-5.5 (<272K context length) | $12.50 | $1.25 | - | $75.00 | - | - | - | - |",
		"| gpt-5.6-sol | $8.00 | $0.80 | $10.00 | $40.00 | $16.00 | $1.60 | $20.00 | $60.00 |",
		"| gpt-6-sol | $4.00 | $0.40 | $5.00 | $20.00 | $8.00 | $0.80 | $10.00 | $30.00 |",
	}, "\n")
}

func TestParseOfficialMarkdownUsesFlagshipTablesOnly(t *testing.T) {
	rates, err := ParseOfficialMarkdown(officialFixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rates["gpt-ignore"]; ok {
		t.Fatal("batch table was applied")
	}
	fast54 := rates["gpt-5.4"].Fast
	if fast54 == nil || fast54.Long != nil || fast54.Short.Input != 5 || fast54.Short.HasCacheWrite {
		t.Fatalf("gpt-5.4 fast long context should stay unpublished: %+v", fast54)
	}
	if rates["gpt-5.4"].Standard.Long == nil || rates["gpt-5.4"].Standard.Long.Output != 22.5 || rates["gpt-5.4"].Standard.Long.HasCacheWrite {
		t.Fatalf("gpt-5.4 standard long context mismatch: %+v", rates["gpt-5.4"].Standard.Long)
	}
	sol := rates["gpt-5.6-sol"].Fast
	if sol == nil || sol.Long == nil || sol.Long.CacheWrite != 20 || !sol.Long.HasCacheWrite {
		t.Fatalf("gpt-5.6-sol fast long context mismatch: %+v", sol)
	}
	if rates["gpt-6-sol"].Standard.Short.Input != 2 {
		t.Fatalf("new model was not imported: %+v", rates["gpt-6-sol"])
	}
}

func TestParseOfficialMarkdownRejectsIncompleteDocument(t *testing.T) {
	_, err := ParseOfficialMarkdown(strings.ReplaceAll(officialFixture(), "### Fast pricing data", "### Flex pricing data"))
	if err == nil {
		t.Fatal("document without a Fast table was accepted")
	}
}

func TestSyncedCatalogPricesNewOfficialModel(t *testing.T) {
	t.Cleanup(ResetOfficialCatalog)
	rates, err := ParseOfficialMarkdown(officialFixture())
	if err != nil {
		t.Fatal(err)
	}
	id, err := OfficialScheduleID(rates, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := UseOfficialCatalog(id, rates); err != nil {
		t.Fatal(err)
	}
	result := EstimateCost(Usage{Model: "gpt-6-sol", InputTokens: 100_000, ServiceTier: "standard"}, nil)
	if result.Status != StatusEstimated || result.ScheduleID != id || result.Amount != 0.2 {
		t.Fatalf("synced model was not priced from the fetched table: %+v", result)
	}
	fastLong := EstimateCost(Usage{Model: "gpt-5.4", InputTokens: LongContextTokens + 1, ServiceTier: "fast"}, nil)
	if fastLong.Status != StatusUnpriced || fastLong.Note != NoteLongContextUnavailable {
		t.Fatalf("unpublished fast long context became a charge: %+v", fastLong)
	}
}
