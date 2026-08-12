package scheduler

import (
	"testing"
)

func TestCronToScheduleDefaults(t *testing.T) {
	tests := []struct {
		name      string
		cron      string
		wantType  string
		wantTime  string
		wantEvery int
	}{
		{name: "checkin default", cron: "0 8 * * *", wantType: "daily", wantTime: "08:00"},
		{name: "balance default hourly", cron: "0 * * * *", wantType: "interval", wantEvery: 1},
		{name: "log cleanup default", cron: "0 6 * * *", wantType: "daily", wantTime: "06:00"},
		{name: "webdav default", cron: "0 */6 * * *", wantType: "interval", wantEvery: 6},
		{name: "daily summary", cron: "58 23 * * *", wantType: "daily", wantTime: "23:58"},
		{name: "six-field daily", cron: "0 30 9 * * *", wantType: "daily", wantTime: "09:30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := CronToSchedule(tt.cron)
			if spec.Type != tt.wantType {
				t.Fatalf("CronToSchedule(%q).Type = %q, want %q", tt.cron, spec.Type, tt.wantType)
			}
			if spec.Time != tt.wantTime {
				t.Fatalf("CronToSchedule(%q).Time = %q, want %q", tt.cron, spec.Time, tt.wantTime)
			}
			if spec.EveryHours != tt.wantEvery {
				t.Fatalf("CronToSchedule(%q).EveryHours = %d, want %d", tt.cron, spec.EveryHours, tt.wantEvery)
			}
			if spec.Cron != tt.cron {
				t.Fatalf("CronToSchedule(%q).Cron = %q, want original bytes preserved", tt.cron, spec.Cron)
			}
			if spec.Version != ScheduleSpecVersion {
				t.Fatalf("CronToSchedule(%q).Version = %d, want %d", tt.cron, spec.Version, ScheduleSpecVersion)
			}
		})
	}
}

func TestCronToScheduleComplexBecomesCustom(t *testing.T) {
	for _, cron := range []string{"0 0 * * 1-5", "*/5 * * * * *", "0 0,12 * * *", "bad cron"} {
		spec := CronToSchedule(cron)
		if spec.Type != "custom" {
			t.Fatalf("CronToSchedule(%q).Type = %q, want custom", cron, spec.Type)
		}
		if spec.Cron != cron {
			t.Fatalf("CronToSchedule(%q).Cron = %q, want original bytes preserved", cron, spec.Cron)
		}
	}
}

func TestScheduleToCronUnchangedSemanticsReturnsOriginalBytes(t *testing.T) {
	tests := []struct {
		name string
		spec ScheduleSpec
		want string
	}{
		{name: "daily unchanged", spec: ScheduleSpec{Version: 1, Type: "daily", Time: "08:00", Cron: "0 8 * * *"}, want: "0 8 * * *"},
		{name: "daily summary unchanged", spec: ScheduleSpec{Version: 1, Type: "daily", Time: "23:58", Cron: "58 23 * * *"}, want: "58 23 * * *"},
		{name: "interval unchanged", spec: ScheduleSpec{Version: 1, Type: "interval", EveryHours: 6, Cron: "0 */6 * * *"}, want: "0 */6 * * *"},
		{name: "custom unchanged", spec: ScheduleSpec{Version: 1, Type: "custom", Cron: "0 0 * * 1-5"}, want: "0 0 * * 1-5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ScheduleToCron(tt.spec)
			if err != nil {
				t.Fatalf("ScheduleToCron() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ScheduleToCron() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScheduleToCronEditedSemanticsGeneratesNormalized(t *testing.T) {
	tests := []struct {
		name string
		spec ScheduleSpec
		want string
	}{
		{name: "daily time edited", spec: ScheduleSpec{Version: 1, Type: "daily", Time: "09:00", Cron: "0 8 * * *"}, want: "0 9 * * *"},
		{name: "interval hours edited", spec: ScheduleSpec{Version: 1, Type: "interval", EveryHours: 4, Cron: "0 */6 * * *"}, want: "0 0 */4 * * *"},
		{name: "daily from scratch", spec: ScheduleSpec{Version: 1, Type: "daily", Time: "23:58"}, want: "58 23 * * *"},
		{name: "interval from scratch", spec: ScheduleSpec{Version: 1, Type: "interval", EveryHours: 2}, want: "0 0 */2 * * *"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ScheduleToCron(tt.spec)
			if err != nil {
				t.Fatalf("ScheduleToCron() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ScheduleToCron() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScheduleSpecValidate(t *testing.T) {
	ok := []ScheduleSpec{
		{Version: 1, Type: "daily", Time: "08:00", Cron: "0 8 * * *"},
		{Version: 1, Type: "interval", EveryHours: 6, Cron: "0 */6 * * *"},
		{Version: 1, Type: "window", WindowStart: "02:00", WindowEnd: "03:30"},
		{Version: 1, Type: "custom", Cron: "0 0 * * 1-5"},
	}
	for _, s := range ok {
		if err := s.Validate(); err != nil {
			t.Fatalf("Validate(%+v) = %v, want nil", s, err)
		}
	}
	bad := []ScheduleSpec{
		{Version: 0, Type: "daily", Time: "08:00"},
		{Version: 1, Type: "daily", Time: "25:00"},
		{Version: 1, Type: "interval", EveryHours: 0},
		{Version: 1, Type: "interval", EveryHours: 25},
		{Version: 1, Type: "window", WindowStart: "13:00", WindowEnd: "08:00"},
		{Version: 1, Type: "custom", Cron: "nope"},
		{Version: 1, Type: "bogus"},
	}
	for _, s := range bad {
		if err := s.Validate(); err == nil {
			t.Fatalf("Validate(%+v) = nil, want error", s)
		}
	}
}

func TestScheduleToCronWindowReturnsPreservedCron(t *testing.T) {
	got, err := ScheduleToCron(ScheduleSpec{Version: 1, Type: "window", WindowStart: "02:00", WindowEnd: "03:30", Cron: "0 3 * * *"})
	if err != nil {
		t.Fatalf("ScheduleToCron(window) error = %v", err)
	}
	if got != "0 3 * * *" {
		t.Fatalf("ScheduleToCron(window) = %q, want preserved cron", got)
	}
}
