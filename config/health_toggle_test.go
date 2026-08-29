package config

import "testing"

// Focused tests for the issue #1027 upstream-account health-monitoring kill
// switches (CHECKIN_ENABLED / BALANCE_REFRESH_ENABLED). Both default to
// enabled (historical behavior) and are stored inverted internally so the
// zero value of RuntimeSettings keeps the default-on semantics.

func TestLoadAccountHealthSwitchesDefaultEnabled(t *testing.T) {
	_, rt := Load(map[string]string{})
	if rt.CheckinDisabled {
		t.Fatal("CheckinDisabled = true, want false default (check-in on)")
	}
	if rt.BalanceRefreshDisabled {
		t.Fatal("BalanceRefreshDisabled = true, want false default (balance refresh on)")
	}
}

func TestLoadAccountHealthSwitchesDisabled(t *testing.T) {
	_, rt := Load(map[string]string{
		"CHECKIN_ENABLED":         "false",
		"BALANCE_REFRESH_ENABLED": "false",
	})
	if !rt.CheckinDisabled {
		t.Fatal("CheckinDisabled = false, want true when CHECKIN_ENABLED=false")
	}
	if !rt.BalanceRefreshDisabled {
		t.Fatal("BalanceRefreshDisabled = false, want true when BALANCE_REFRESH_ENABLED=false")
	}
}

func TestLoadAccountHealthSwitchesExplicitEnabled(t *testing.T) {
	_, rt := Load(map[string]string{
		"CHECKIN_ENABLED":         "true",
		"BALANCE_REFRESH_ENABLED": "true",
	})
	if rt.CheckinDisabled {
		t.Fatal("CheckinDisabled = true, want false when CHECKIN_ENABLED=true")
	}
	if rt.BalanceRefreshDisabled {
		t.Fatal("BalanceRefreshDisabled = true, want false when BALANCE_REFRESH_ENABLED=true")
	}
}
