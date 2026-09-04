package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVeloeraAdapter_Detect(t *testing.T) {
	v := &VeloeraAdapter{BaseAdapter: NewBaseAdapter("veloera")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Detect requires HTTP probe; returns false for unreachable URL
	ok, err := v.Detect(ctx, unreachableBaseURL(t))
	if err != nil {
		t.Errorf("Detect should not return error on probe failure: %v", err)
	}
	if ok {
		t.Error("Detect should return false for unreachable URL")
	}
}

func TestVeloeraAdapter_Headers(t *testing.T) {
	// veloeraHeaders sets Authorization + Veloera-User + New-API-User + User-id
	id := 5
	h := veloeraHeaders("token", &id)

	if h["Authorization"] != "Bearer token" {
		t.Errorf("Authorization: %q", h["Authorization"])
	}
	if h["Veloera-User"] != "5" {
		t.Errorf("Veloera-User: %q", h["Veloera-User"])
	}
	if h["New-API-User"] != "5" {
		t.Errorf("New-API-User: %q", h["New-API-User"])
	}
	if h["User-id"] != "5" {
		t.Errorf("User-id: %q", h["User-id"])
	}
	// Veloera only sets 3 user headers (not 7 like NewApi)
	if len(h) != 4 { // Authorization + 3 user headers
		t.Errorf("expected 4 headers total, got %d: %v", len(h), h)
	}

	// nil userID
	h2 := veloeraHeaders("token", nil)
	if h2["Authorization"] != "Bearer token" {
		t.Errorf("Authorization with nil: %q", h2["Authorization"])
	}
	if _, ok := h2["Veloera-User"]; ok {
		t.Error("Veloera-User should not be set with nil userID")
	}
}

func TestVeloeraAdapter_Checkin(t *testing.T) {
	v := &VeloeraAdapter{BaseAdapter: NewBaseAdapter("veloera")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cr, err := v.Checkin(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err != nil {
		t.Errorf("Checkin should not error: %v", err)
	}
	if cr.Success {
		t.Error("Checkin on unreachable URL should fail")
	}
}

func TestVeloeraAdapter_CheckinWithUserID(t *testing.T) {
	v := &VeloeraAdapter{BaseAdapter: NewBaseAdapter("veloera")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	id := 1
	cr, err := v.Checkin(ctx, unreachableBaseURL(t), "token", &id, nil)
	if err != nil {
		t.Errorf("Checkin with userID should not error: %v", err)
	}
	if cr.Success {
		t.Error("Checkin on unreachable URL should fail")
	}
}

// TestVeloeraAdapter_GetBalance_1MDivisor reads a real payload and checks the
// divisor its name claims: Veloera divides by 1,000,000 (NOT one-api's 500,000)
// and its `quota` is the TOTAL, so Balance = quota - used.
//
// It used to point GetBalance at an unreachable URL and assert Balance == 0, which
// is what any divisor returns for a failed fetch — the 1M divisor was never
// exercised here (TestVeloeraAdapter_DivisorIs1M does the arithmetic on literals),
// and it pinned the adapter reporting a failed fetch as a legitimate zero balance.
func TestVeloeraAdapter_GetBalance_1MDivisor(t *testing.T) {
	v := &VeloeraAdapter{BaseAdapter: NewBaseAdapter("veloera")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":2000000,"used_quota":500000}}`))
	}))
	defer srv.Close()

	bi, err := v.GetBalance(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Fatalf("GetBalance against an answering upstream: %v", err)
	}
	if bi == nil {
		t.Fatal("GetBalance returned nil BalanceInfo with no error")
	}
	// 1M divisor, quota is total: Quota = 2.0, Used = 0.5, Balance = 1.5.
	// At one-api's 500k these would be 4.0 / 1.0 / 3.0.
	if bi.Quota != 2.0 || bi.Used != 0.5 || bi.Balance != 1.5 {
		t.Errorf("balance = (%g, %g, %g), want (1.5, 0.5, 2.0) for a 1M divisor",
			bi.Balance, bi.Used, bi.Quota)
	}
}

func TestVeloeraAdapter_DivisorIs1M(t *testing.T) {
	// Veloera uses 1,000,000 divisor, NOT 500,000
	quota := 2000000.0
	used := 500000.0

	balance := (quota - used) / 1000000
	quotaUSD := quota / 1000000
	usedUSD := used / 1000000

	if balance != 1.5 {
		t.Errorf("Veloera balance: %f, want 1.5", balance)
	}
	if quotaUSD != 2.0 {
		t.Errorf("Veloera quotaUSD: %f, want 2.0", quotaUSD)
	}
	if usedUSD != 0.5 {
		t.Errorf("Veloera usedUSD: %f, want 0.5", usedUSD)
	}

	// Compare with NewApi divisor (500000)
	newApiBalance := (quota - used) / 500000 // 3.0
	if newApiBalance == balance {
		t.Error("Veloera 1M divisor should differ from NewApi 500K divisor for same inputs")
	}
}

func TestVeloeraAdapter_GetModels(t *testing.T) {
	v := &VeloeraAdapter{BaseAdapter: NewBaseAdapter("veloera")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models, err := v.GetModels(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Error("GetModels on unreachable should surface a fetch error")
	}
	if len(models) != 0 {
		t.Error("GetModels on unreachable should return empty models")
	}
}
