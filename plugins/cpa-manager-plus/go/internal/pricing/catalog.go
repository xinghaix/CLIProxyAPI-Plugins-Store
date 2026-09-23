package pricing

import "sync/atomic"

// OfficialPricingURL is the machine-readable OpenAI API price page.
// Only its flagship Standard and Fast tables are applied.
const OfficialPricingURL = "https://developers.openai.com/api/docs/pricing.md"

type officialCatalog struct {
	id    string
	rates map[string]ModelSchedule
}

var currentCatalog atomic.Pointer[officialCatalog]

func init() {
	currentCatalog.Store(compiledCatalog())
}

func compiledCatalog() *officialCatalog {
	return &officialCatalog{id: ScheduleID, rates: cloneRates(officialRates)}
}

func activeCatalog() *officialCatalog {
	return currentCatalog.Load()
}

// UseOfficialCatalog replaces the in-memory schedule used for estimates.
// An empty catalog is rejected so a bad sync cannot wipe the last good table.
func UseOfficialCatalog(id string, rates map[string]ModelSchedule) error {
	if id == "" || len(rates) == 0 {
		return errEmptyCatalog
	}
	currentCatalog.Store(&officialCatalog{id: id, rates: cloneRates(rates)})
	return nil
}

// ResetOfficialCatalog restores the compiled seed. Tests use it to avoid
// leaking a synced table into later cases.
func ResetOfficialCatalog() {
	currentCatalog.Store(compiledCatalog())
}

func cloneRates(rates map[string]ModelSchedule) map[string]ModelSchedule {
	out := make(map[string]ModelSchedule, len(rates))
	for model, schedule := range rates {
		out[model] = cloneSchedule(schedule)
	}
	return out
}

func cloneSchedule(schedule ModelSchedule) ModelSchedule {
	schedule.Standard.Long = cloneTokenRates(schedule.Standard.Long)
	if schedule.Fast != nil {
		fast := *schedule.Fast
		fast.Long = cloneTokenRates(schedule.Fast.Long)
		schedule.Fast = &fast
	}
	return schedule
}

func cloneTokenRates(rates *TokenRates) *TokenRates {
	if rates == nil {
		return nil
	}
	copy := *rates
	return &copy
}
