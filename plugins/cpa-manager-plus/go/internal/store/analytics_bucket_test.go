package store

import (
	"testing"
	"time"
)

func TestAnalyticsBucketMSAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	tests := []struct {
		name        string
		timestampMS int64
		granularity string
		wantMS      int64
	}{
		{
			name:        "hour before spring transition",
			timestampMS: time.Date(2026, time.March, 8, 6, 30, 0, 0, time.UTC).UnixMilli(),
			granularity: "hour",
			wantMS:      time.Date(2026, time.March, 8, 6, 0, 0, 0, time.UTC).UnixMilli(),
		},
		{
			name:        "hour after spring transition",
			timestampMS: time.Date(2026, time.March, 8, 7, 30, 0, 0, time.UTC).UnixMilli(),
			granularity: "hour",
			wantMS:      time.Date(2026, time.March, 8, 7, 0, 0, 0, time.UTC).UnixMilli(),
		},
		{
			name:        "local day",
			timestampMS: time.Date(2026, time.March, 8, 18, 0, 0, 0, time.UTC).UnixMilli(),
			granularity: "day",
			wantMS:      time.Date(2026, time.March, 8, 5, 0, 0, 0, time.UTC).UnixMilli(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := AnalyticsBucketMS(test.timestampMS, test.granularity, location); got != test.wantMS {
				t.Fatalf("bucket = %d, want %d", got, test.wantMS)
			}
		})
	}
}

func TestAnalyticsBucketMSOffsetTransitions(t *testing.T) {
	tests := []struct {
		name, zone, instant, want string
	}{
		{"New York first 01 hour", "America/New_York", "2026-11-01T05:30:12.345Z", "2026-11-01T05:00:00Z"},
		{"New York second 01 hour", "America/New_York", "2026-11-01T06:30:12.345Z", "2026-11-01T06:00:00Z"},
		{"Kathmandu quarter hour offset", "Asia/Kathmandu", "2026-11-01T00:45:12.345Z", "2026-11-01T00:15:00Z"},
		{"Kathmandu previous UTC day", "Asia/Kathmandu", "2026-11-01T00:05:00Z", "2026-10-31T23:15:00Z"},
		{"Lord Howe before fall", "Australia/Lord_Howe", "2026-04-04T14:59:59.999Z", "2026-04-04T14:00:00Z"},
		{"Lord Howe fall boundary", "Australia/Lord_Howe", "2026-04-04T15:00:00Z", "2026-04-04T15:00:00Z"},
		{"Lord Howe repeated half hour", "Australia/Lord_Howe", "2026-04-04T15:15:12.345Z", "2026-04-04T15:00:00Z"},
		{"Lord Howe full hour after fall", "Australia/Lord_Howe", "2026-04-04T15:45:00Z", "2026-04-04T15:30:00Z"},
		{"Lord Howe before spring", "Australia/Lord_Howe", "2026-10-03T15:29:59.999Z", "2026-10-03T14:30:00Z"},
		{"Lord Howe spring boundary", "Australia/Lord_Howe", "2026-10-03T15:30:00Z", "2026-10-03T15:30:00Z"},
		{"Lord Howe shortened spring hour", "Australia/Lord_Howe", "2026-10-03T15:45:12.345Z", "2026-10-03T15:30:00Z"},
		{"Lord Howe full hour after spring", "Australia/Lord_Howe", "2026-10-03T16:15:00Z", "2026-10-03T16:00:00Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			location, err := time.LoadLocation(test.zone)
			if err != nil {
				t.Fatal(err)
			}
			instant, err := time.Parse(time.RFC3339Nano, test.instant)
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339Nano, test.want)
			if err != nil {
				t.Fatal(err)
			}
			got := AnalyticsBucketMS(instant.UnixMilli(), "hour", location)
			if got != want.UnixMilli() {
				t.Fatalf("bucket of %s = %s, want %s", instant.In(location), time.UnixMilli(got).In(location), want.In(location))
			}
			if again := AnalyticsBucketMS(got, "hour", location); again != got {
				t.Fatalf("bucket is not idempotent: %d -> %d", got, again)
			}
		})
	}
}

func TestAnalyticsTimelineRepeatedHour(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, time.November, 1, 5, 30, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	result := aggregate([]eventRow{{TimestampMS: first.UnixMilli()}, {TimestampMS: second.UnixMilli()}}, nil, AnalyticsRequest{Location: location, Granularity: "hour", Limit: 2})
	timeline := result["timeline"].([]map[string]any)
	if len(timeline) != 2 {
		t.Fatalf("timeline = %#v, want two distinct hours", timeline)
	}
	for i, bucket := range timeline {
		want := first.Truncate(time.Hour).Add(time.Duration(i) * time.Hour).UnixMilli()
		if bucket["bucket_ms"] != want || bucket["calls"] != int64(1) {
			t.Fatalf("timeline[%d] = %#v, want bucket %d with one call", i, bucket, want)
		}
	}
	if total := result["summary"].(map[string]any)["calls"]; total != int64(2) {
		t.Fatalf("total calls = %v, want 2", total)
	}
	heatmap := result["heatmap"].([]map[string]any)
	if len(heatmap) != 1 || heatmap[0]["calls"] != int64(2) {
		t.Fatalf("heatmap = %#v, want both calls in the same local clock hour", heatmap)
	}
	for _, instant := range []time.Time{first, second} {
		want := time.Date(2026, time.November, 1, 4, 0, 0, 0, time.UTC).UnixMilli()
		if got := AnalyticsBucketMS(instant.UnixMilli(), "day", location); got != want {
			t.Fatalf("day bucket = %d, want local midnight %d", got, want)
		}
	}
}

func TestAnalyticsBucketMSShanghaiDay(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 2026-03-08 01:30 CST = 2026-03-07 17:30 UTC → local day Mar 8 00:00 CST
	ts := time.Date(2026, time.March, 7, 17, 30, 0, 0, time.UTC).UnixMilli()
	want := time.Date(2026, time.March, 8, 0, 0, 0, 0, location).UnixMilli()
	if got := AnalyticsBucketMS(ts, "day", location); got != want {
		t.Fatalf("bucket = %d, want %d", got, want)
	}
}

func TestResolveAnalyticsLocation(t *testing.T) {
	if got := ResolveAnalyticsLocation("", "Asia/Shanghai"); got.String() != "Asia/Shanghai" {
		t.Fatalf("got %s", got)
	}
	if got := ResolveAnalyticsLocation("Not/AZone", "UTC"); got.String() != "UTC" {
		t.Fatalf("got %s", got)
	}
}
