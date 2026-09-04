package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAntigravityAdapter_Detect(t *testing.T) {
	a := &AntigravityAdapter{StandardAdapter: NewStandardAdapter("antigravity")}

	ctx := context.Background()

	tests := []struct {
		url     string
		matches bool
	}{
		{"https://antigravity.googleapis.com", true},
		{"https://antigravity.googleapis.com/v1/models", true},
		{"https://antigravity.example.com/v1/models", false},
		{"https://example.com/antigravity/v1", false},
		{"https://api.openai.com", false},
	}
	for _, tt := range tests {
		ok, err := a.Detect(ctx, tt.url)
		if err != nil {
			t.Errorf("Detect(%q) error: %v", tt.url, err)
			continue
		}
		if ok != tt.matches {
			t.Errorf("Detect(%q) = %v, want %v", tt.url, ok, tt.matches)
		}
	}
}

func TestAntigravityAdapter_LoginUnspported(t *testing.T) {
	a := &AntigravityAdapter{StandardAdapter: NewStandardAdapter("antigravity")}
	ctx := context.Background()

	lr, err := a.Login(ctx, "http://x", "u", "p", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if lr.Success {
		t.Error("Antigravity login should return Success=false")
	}
}

func TestAntigravityAdapter_CheckinUnspported(t *testing.T) {
	a := &AntigravityAdapter{StandardAdapter: NewStandardAdapter("antigravity")}
	ctx := context.Background()

	cr, err := a.Checkin(ctx, "http://x", "t", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cr.Success {
		t.Error("Antigravity checkin should return Success=false")
	}
}

func TestAntigravityAdapter_GetBalanceZero(t *testing.T) {
	a := &AntigravityAdapter{StandardAdapter: NewStandardAdapter("antigravity")}
	ctx := context.Background()

	bi, err := a.GetBalance(ctx, "http://x", "t", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if bi.Balance != 0 || bi.Used != 0 || bi.Quota != 0 {
		t.Error("Antigravity balance should be all zeros")
	}
}

// TestAntigravityAdapter_GetModels pins both directions of the model-discovery
// contract. A failed fetch must come back as an error so
// service.classifyModelRefreshError can name the reason; returning an empty list
// instead made an unreachable or unauthorized upstream indistinguishable from "no
// models", which reaches the operator as the single word empty_models (#1232
// family). An upstream that answers with no models is still a legitimate empty.
func TestAntigravityAdapter_GetModels(t *testing.T) {
	a := &AntigravityAdapter{StandardAdapter: NewStandardAdapter("antigravity")}
	ctx := context.Background()

	models, err := a.GetModels(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Error("GetModels must report an unreachable upstream, not an empty model list")
	}
	if len(models) != 0 {
		t.Errorf("expected no models when the fetch failed, got %v", models)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"gemini-2.5-pro":{},"claude-sonnet-4":{}}}`))
	}))
	defer srv.Close()
	models, err = a.GetModels(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Fatalf("GetModels against an answering upstream: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %v", models)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{}}`))
	}))
	defer empty.Close()
	models, err = a.GetModels(ctx, empty.URL, "token", nil, nil)
	if err != nil {
		t.Errorf("an answered listing with no models is not an error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected zero models, got %v", models)
	}
}

func TestExtractAntigravityModelNames(t *testing.T) {
	// Object form
	obj := map[string]interface{}{
		"models": map[string]interface{}{
			"gpt-4":         map[string]interface{}{},
			"claude-3-opus": map[string]interface{}{},
			"  ":            map[string]interface{}{}, // whitespace name, should be filtered
		},
	}
	names := extractAntigravityModelNames(obj)
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}
	hasGPT4 := false
	hasClaude := false
	for _, n := range names {
		if n == "gpt-4" {
			hasGPT4 = true
		}
		if n == "claude-3-opus" {
			hasClaude = true
		}
	}
	if !hasGPT4 || !hasClaude {
		t.Errorf("missing expected models: %v", names)
	}

	// Array form with id/name fields
	arr := map[string]interface{}{
		"models": []interface{}{
			map[string]interface{}{"id": "model-a", "name": "ignored"},
			map[string]interface{}{"name": "model-b"},
			"model-c",
		},
	}
	names2 := extractAntigravityModelNames(arr)
	if len(names2) != 3 {
		t.Fatalf("expected 3 names, got %d: %v", len(names2), names2)
	}

	// No models key
	names3 := extractAntigravityModelNames(map[string]interface{}{})
	if len(names3) != 0 {
		t.Errorf("expected empty, got %v", names3)
	}

	// Empty models
	names4 := extractAntigravityModelNames(map[string]interface{}{"models": map[string]interface{}{}})
	if len(names4) != 0 {
		t.Errorf("expected empty, got %v", names4)
	}
}
