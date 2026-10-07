package store

import (
	"strings"
	"time"
)

// AnalyticsBucketMS resolves an event timestamp to the start of its local
// analytics hour or day bucket.
//
// Portions adapted from CPA-Manager-Plus (MIT, Seakee) analytics_bucket.go.
func AnalyticsBucketMS(timestampMS int64, granularity string, location *time.Location) int64 {
	if location == nil {
		location = time.UTC
	}
	tm := time.UnixMilli(timestampMS).In(location)
	if granularity == "day" {
		return time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, location).UnixMilli()
	}
	// Subtract elapsed minutes, keeping the event's occurrence of a repeated hour.
	start := tm.Add(-time.Duration(tm.Minute())*time.Minute - time.Duration(tm.Second())*time.Second - time.Duration(tm.Nanosecond()))
	// A half-hour DST change can start an hour at :30; do not cross its offset boundary.
	if zoneStart, _ := tm.ZoneBounds(); !zoneStart.IsZero() && start.Before(zoneStart) {
		start = zoneStart
	}
	return start.UnixMilli()
}

// ResolveAnalyticsLocation picks the first valid IANA timezone name, else Local.
func ResolveAnalyticsLocation(names ...string) *time.Location {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.Local
}
