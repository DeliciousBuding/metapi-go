package service

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func captureWarnLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

// TestParseResinURLRedactsCredentials pins audit L14: a malformed or token-less
// RESIN_URL may embed user:password@host and must never be logged verbatim.
func TestParseResinURLRedactsCredentials(t *testing.T) {
	logs := captureWarnLogs(t)

	cfg := &config.Config{ResinURL: "http://user:super-secret@resin.local:2260/"}
	host, token, ok := parseResinURL(cfg)
	if ok || host != "" || token != "" {
		t.Fatalf("missing token should fall back: host=%q token=%q ok=%v", host, token, ok)
	}
	assertNoSecret(t, logs.String())
	if !strings.Contains(logs.String(), "resin.local:2260") {
		t.Fatalf("log should keep the host for diagnosis: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "missing_token") {
		t.Fatalf("log should record the error class: %s", logs.String())
	}
}

func TestParseResinURLInvalidDoesNotLogRawValue(t *testing.T) {
	logs := captureWarnLogs(t)

	cfg := &config.Config{ResinURL: "http://user:super-secret@resin.local:bad port/token"}
	_, _, ok := parseResinURL(cfg)
	if ok {
		t.Fatal("invalid URL must fall back to direct")
	}
	assertNoSecret(t, logs.String())
	if !strings.Contains(logs.String(), "invalid_url") {
		t.Fatalf("log should record the invalid_url class: %s", logs.String())
	}
}

func assertNoSecret(t *testing.T, logs string) {
	t.Helper()
	for _, secret := range []string{"super-secret", "user:", "@"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs leaked %q: %s", secret, logs)
		}
	}
}
