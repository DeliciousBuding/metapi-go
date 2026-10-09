package store

import (
	"testing"
	"time"
)

func TestDownstreamQuotaWindows(t *testing.T) {
	now := time.Date(2026, 3, 8, 20, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		period     DownstreamQuotaPeriod
		zone       string
		start, end string
	}{
		{"rolling minutes", DownstreamQuotaPeriod{Type: "past_duration", PastDuration: &DownstreamQuotaDuration{Value: 5, Unit: "minute"}}, "UTC", "2026-03-08T20:25:00Z", "2026-03-08T20:30:00.001Z"},
		{"calendar DST day", DownstreamQuotaPeriod{Type: "calendar_duration", CalendarDuration: &DownstreamQuotaDuration{Unit: "day"}}, "America/New_York", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z"},
		{"calendar month", DownstreamQuotaPeriod{Type: "calendar_duration", CalendarDuration: &DownstreamQuotaDuration{Unit: "month"}}, "Asia/Hong_Kong", "2026-02-28T16:00:00Z", "2026-03-31T16:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := DownstreamQuota{Period: tc.period, Timezone: tc.zone}
			start, end, err := q.Window(now)
			if err != nil {
				t.Fatal(err)
			}
			a, _ := time.Parse(time.RFC3339Nano, tc.start)
			b, _ := time.Parse(time.RFC3339Nano, tc.end)
			if start != a.UnixMilli() || end != b.UnixMilli() {
				t.Fatalf("window=%s..%s", time.UnixMilli(start).UTC(), time.UnixMilli(end).UTC())
			}
		})
	}
}

func TestDownstreamAccessPolicyStrictnessAndOrderedMapping(t *testing.T) {
	for _, raw := range []string{`{"unknown":true}`, `{"modelMappings":[{"from":"(?=foo)","to":"bar"}]}`, `{} {}`, `{"allowedUpstreamChannelIds":[-1]}`, `{"quota":{"requests":1,"period":{"type":"past_duration","pastDuration":{"value":9223372036854775807,"unit":"day"}}}}`} {
		if _, err := ParseDownstreamAccessPolicy(raw); err == nil {
			t.Fatalf("accepted invalid policy: %s", raw)
		}
	}
	p, err := ParseDownstreamAccessPolicy(`{"allowedUpstreamChannelIds":[],"modelIds":["first"],"modelMappings":[{"from":"(?i)alias-.*","to":"first"},{"from":"*","to":"second"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.MapModel("ALIAS-a") != "first" || !p.AllowsModel("ALIAS-a") || p.AllowsModel("other") {
		t.Fatal("ordered mapping or mapped exact allowlist violated")
	}
	if p.AllowsUpstreamChannel(1) || p.AllowsUpstreamChannel(0) {
		t.Fatal("strict empty channel list widened")
	}
	var unrestricted *DownstreamAccessPolicy
	if !unrestricted.AllowsUpstreamChannel(0) || !unrestricted.AllowsModel("any") {
		t.Fatal("legacy nil policy changed")
	}
}
