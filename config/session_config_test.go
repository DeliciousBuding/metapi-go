package config

import "testing"

// ---- #1034 session-model config ----

func TestSessionCookieSecureNormalization(t *testing.T) {
	cases := map[string]string{
		"":       "auto",
		"auto":   "auto",
		"AUTO":   "auto",
		"true":   "true",
		"TRUE":   "true",
		"1":      "true",
		"yes":    "true",
		"false":  "false",
		"0":      "false",
		"no":     "false",
		"banana": "auto",
		" true ": "true",
	}
	for in, want := range cases {
		if got := normalizeSessionCookieSecure(in); got != want {
			t.Errorf("normalizeSessionCookieSecure(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSessionDefaultsAreSafe(t *testing.T) {
	if DefaultAdminSessionTTLMinutes <= 0 || DefaultAdminSessionTTLMinutes > 24*60 {
		t.Fatalf("DefaultAdminSessionTTLMinutes = %d, want a sane bounded window", DefaultAdminSessionTTLMinutes)
	}
	if DefaultAdminSessionCookieSecure != "auto" {
		t.Fatalf("DefaultAdminSessionCookieSecure = %q, want auto", DefaultAdminSessionCookieSecure)
	}
	if DefaultAuthRateLimitRPS <= 0 || DefaultAuthRateLimitBurst <= 0 {
		t.Fatal("auth rate-limit defaults must be active (login is the master-token surface)")
	}
}
