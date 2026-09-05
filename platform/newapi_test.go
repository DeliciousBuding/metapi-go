package platform

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// --- NewApiAdapter ---

func TestNewApiAdapter_Detect(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Detect requires HTTP probe; will fail and return false for non-existent URLs
	ok, err := n.Detect(ctx, unreachableBaseURL(t))
	if err != nil {
		t.Errorf("Detect should not return error (should return false on probe failure): %v", err)
	}
	if ok {
		t.Error("Detect should return false for unreachable URL")
	}
}

func TestNewApiAdapter_ExtractLikelyUserIDs_RE2Boundaries(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
	tests := []struct {
		name    string
		payload string
		want    []int
	}{
		{name: "underscore before delimiter", payload: `prefix_8765432|suffix`, want: []int{8765432}},
		{name: "underscore at end", payload: `prefix_1234`, want: []int{1234}},
		{name: "named user before delimiter", payload: `username: 567890;`, want: []int{567890}},
		{name: "named id at end", payload: `uid=4321`, want: []int{4321}},
		{name: "adjacent underscore ids", payload: `prefix_1234_5678`, want: []int{1234, 5678}},
		{name: "reject underscore nine digits", payload: `prefix_123456789|suffix`, want: nil},
		{name: "reject named nine digits", payload: `user_id=123456789;`, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := "session=" + base64.RawStdEncoding.EncodeToString([]byte(tt.payload))
			got := n.extractLikelyUserIDs(token)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("extractLikelyUserIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewApiAdapter_UserIDHeaders(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}

	// nil userID
	h := n.userIDHeaders(nil)
	if len(h) != 0 {
		t.Errorf("headers with nil userID should be empty: %v", h)
	}

	// With userID
	id := 42
	h2 := n.userIDHeaders(&id)
	if len(h2) != 7 {
		t.Fatalf("expected 7 headers, got %d: %v", len(h2), h2)
	}
	if h2["New-API-User"] != "42" {
		t.Errorf("New-API-User: %q", h2["New-API-User"])
	}
	if h2["Veloera-User"] != "42" {
		t.Errorf("Veloera-User: %q", h2["Veloera-User"])
	}
	if h2["voapi-user"] != "42" {
		t.Errorf("voapi-user: %q", h2["voapi-user"])
	}
	if h2["User-id"] != "42" {
		t.Errorf("User-id: %q", h2["User-id"])
	}
	if h2["X-User-Id"] != "42" {
		t.Errorf("X-User-Id: %q", h2["X-User-Id"])
	}
	if h2["Rix-Api-User"] != "42" {
		t.Errorf("Rix-Api-User: %q", h2["Rix-Api-User"])
	}
	if h2["neo-api-user"] != "42" {
		t.Errorf("neo-api-user: %q", h2["neo-api-user"])
	}
}

func TestNewApiAdapter_AuthHeaders(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}

	id := 7
	h := n.authHeaders("mytoken", &id)

	if h["Authorization"] != "Bearer mytoken" {
		t.Errorf("Authorization: %q", h["Authorization"])
	}
	if h["New-API-User"] != "7" {
		t.Errorf("New-API-User: %q", h["New-API-User"])
	}
}

func TestNewApiAdapter_TryDecodeUserID(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}

	// Not a JWT
	if id := n.tryDecodeUserID("plain-token"); id != nil {
		t.Errorf("plain token should return nil: %v", id)
	}

	// Invalid JWT
	if id := n.tryDecodeUserID("a.b.c"); id != nil {
		t.Errorf("invalid JWT should return nil: %v", id)
	}

	// Empty token
	if id := n.tryDecodeUserID(""); id != nil {
		t.Errorf("empty token should return nil: %v", id)
	}
}

func TestNewApiAdapter_ParseBalance(t *testing.T) {
	// Normal case: quota=remaining, balance=quota/500000
	// quota=500000, used_quota=100000 => quotaUSD=1, usedUSD=0.2, total=1.2
	data := map[string]interface{}{
		"quota":      float64(500000),
		"used_quota": float64(100000),
	}
	b := parseOneApiStyleBalance(data, 500000, true)
	if b.Balance != 1.0 {
		t.Errorf("Balance (quotaUSD): %f, want 1.0", b.Balance)
	}
	if b.Used != 0.2 {
		t.Errorf("Used (usedUSD): %f, want 0.2", b.Used)
	}
	if b.Quota != 1.2 {
		t.Errorf("Quota (totalUSD): %f, want 1.2", b.Quota)
	}

	// With today_income
	data2 := map[string]interface{}{
		"quota":        float64(1000000),
		"used_quota":   float64(500000),
		"today_income": float64(100000),
	}
	b2 := parseOneApiStyleBalance(data2, 500000, true)
	if b2.Balance != 2.0 {
		t.Errorf("Balance: %f", b2.Balance)
	}
	if b2.TodayIncome == nil || *b2.TodayIncome != 0.2 {
		t.Errorf("TodayIncome: %v", b2.TodayIncome)
	}

	// Empty data
	data3 := map[string]interface{}{}
	b3 := parseOneApiStyleBalance(data3, 500000, true)
	if b3.Balance != 0 || b3.Used != 0 || b3.Quota != 0 {
		t.Error("empty data should return all zeros")
	}
}

func TestNewApiAdapter_BalanceQuotaIsRemaining(t *testing.T) {
	// NewApi model A: quota = remaining, total = quota + used
	data := map[string]interface{}{
		"quota":      float64(400000),
		"used_quota": float64(100000),
	}
	b := parseOneApiStyleBalance(data, 500000, true)

	// quotaUSD = 400000/500000 = 0.8
	// usedUSD = 100000/500000 = 0.2
	// totalUSD = 0.8 + 0.2 = 1.0
	if b.Balance != 0.8 {
		t.Errorf("Balance (remaining converted): %f, want 0.8", b.Balance)
	}
	if b.Quota != 1.0 {
		t.Errorf("Quota (total): %f, want 1.0", b.Quota)
	}
}

func TestNewApiAdapter_DefaultUnsupportedMethods(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
	// GetUserInfo may cookie-probe many user ids against unreachable hosts.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// GetUserInfo on unreachable URL should return nil, nil
	ui, err := n.GetUserInfo(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ui != nil {
		t.Error("GetUserInfo should return nil for unreachable")
	}

	// GetSiteAnnouncements on unreachable URL
	anns, err := n.GetSiteAnnouncements(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Fatal("GetSiteAnnouncements on unreachable URL should return an error")
	}
	if len(anns) != 0 {
		t.Error("GetSiteAnnouncements on unreachable should return empty")
	}
}

func TestNewApiAdapter_GetSiteAnnouncementsEnvelope(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantErrContains string
		wantContent     string
	}{
		{
			name:            "failure envelope surfaces upstream message",
			body:            `{"success":false,"message":"notice access denied","data":"must not persist"}`,
			wantErrContains: "notice access denied",
		},
		{
			name: "successful empty envelope returns no announcements",
			body: `{"success":true}`,
		},
		{
			name:        "valid string notice",
			body:        `{"success":true,"data":"  Scheduled maintenance  "}`,
			wantContent: "Scheduled maintenance",
		},
		{
			name:            "missing success is not a successful envelope",
			body:            `{}`,
			wantErrContains: "missing boolean success",
		},
		{
			name:            "object data is rejected",
			body:            `{"success":true,"data":{"content":"must not persist"}}`,
			wantErrContains: "data must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/notice" {
					t.Fatalf("path = %q, want /api/notice", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
			anns, err := n.GetSiteAnnouncements(context.Background(), srv.URL, "secret-token", nil, nil)
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErrContains)
				}
				if len(anns) != 0 {
					t.Fatalf("announcements = %#v, want empty on error", anns)
				}
				if strings.Contains(err.Error(), "secret-token") {
					t.Fatalf("error leaked access token: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetSiteAnnouncements() error = %v", err)
			}
			if tt.wantContent == "" {
				if len(anns) != 0 {
					t.Fatalf("announcements = %#v, want empty", anns)
				}
				return
			}
			if len(anns) != 1 || anns[0].Content != tt.wantContent {
				t.Fatalf("announcements = %#v, want one with content %q", anns, tt.wantContent)
			}
		})
	}
}

// --- Gob decoding ---

func TestDecodeGobSignedInt(t *testing.T) {
	// Small positive (single byte)
	result := decodeGobSignedInt([]byte{0x04}) // unsigned=4, even -> zigzag: 4>>1 = 2
	if result != 2 {
		t.Errorf("single byte 0x04: %d, want 2", result)
	}

	// Zero
	result2 := decodeGobSignedInt([]byte{0x00})
	if result2 != 0 {
		t.Errorf("zero: %d", result2)
	}

	// Empty
	result3 := decodeGobSignedInt([]byte{})
	if result3 != 0 {
		t.Errorf("empty: %d", result3)
	}

	// Value too large (rejected by > 10_000_000 check)
	result4 := decodeGobSignedInt([]byte{0x20}) // unsigned=32, even: 16
	if result4 != 16 {
		t.Errorf("0x20: %d, want 16", result4)
	}
}

func TestExtractGobFieldInts(t *testing.T) {
	// Build a gob-like payload with 'id' field
	// Field name "id" + 0x03 + "int" + 0x04 + length=2 + 0x00 + value=0x0A (zigzag: 5)
	marker := []byte{'i', 'd', 0x03, 'i', 'n', 't', 0x04}
	payload := make([]byte, 0)
	payload = append(payload, marker...)
	payload = append(payload, 0x02) // encoded_length = 2
	payload = append(payload, 0x00) // delimiter
	payload = append(payload, 0x0A) // value byte (unsigned=10, even -> zigzag >> 1 = 5)

	ids := extractGobFieldInts(payload, "id")
	if len(ids) != 1 || ids[0] != 5 {
		t.Errorf("extractGobFieldInts: %v, want [5]", ids)
	}

	// Empty payload
	ids2 := extractGobFieldInts([]byte{}, "id")
	if len(ids2) != 0 {
		t.Errorf("empty payload should return empty: %v", ids2)
	}

	// No match
	ids3 := extractGobFieldInts([]byte("random data"), "id")
	if len(ids3) != 0 {
		t.Errorf("no match should return empty: %v", ids3)
	}
}

func TestIndexOf(t *testing.T) {
	data := []byte("hello world")
	if i := indexOf(data, []byte("world"), 0); i != 6 {
		t.Errorf("indexOf 'world': %d, want 6", i)
	}
	if i := indexOf(data, []byte("missing"), 0); i != -1 {
		t.Errorf("indexOf 'missing': %d, want -1", i)
	}
	if i := indexOf(data, []byte("hello"), 1); i != -1 {
		t.Errorf("indexOf 'hello' from 1: %d, want -1", i)
	}
}

// --- ShouldFallbackToCookieCheckin ---

func TestShouldFallbackToCookieCheckin(t *testing.T) {
	tests := []struct {
		msg      string
		fallback bool
	}{
		{"unexpected token", true},
		{"<html>error</html>", true},
		{"new-api-user header required", true},
		{"access token expired", true},
		{"unauthorized", true},
		{"forbidden", true},
		{"not login", true},
		{"未登录", true},
		{"normal error message", false},
		{"checkin success", false},
	}
	for _, tt := range tests {
		if got := shouldFallbackToCookieCheckin(tt.msg); got != tt.fallback {
			t.Errorf("shouldFallbackToCookieCheckin(%q) = %v, want %v", tt.msg, got, tt.fallback)
		}
	}
}

// --- the retired isMissingCheckinEndpointMessage copy ---

// The gate at the end of NewApiAdapter.Checkin used to consult a private
// seven-pattern copy of the check-in vocabulary. Five of those seven patterns
// could not reach it: the caller returns early unless shouldFallbackToCookieCheckin
// accepted the message, and only "invalid url (POST /api/user/checkin)" and the
// "HTTP 404 + /api/user/checkin" pair appear in both lists. This pins that
// equivalence, so replacing the copy with the shared narrow predicate
// (IsCheckinEndpointAbsent) is provably not a behavior change on any input the
// gate can see — and pins the unreachability itself, because if a future edit
// lets one of the five through, the retirement has to be re-examined.
func TestCheckinEndpointAbsentGateEqualsRetiredCopyOnReachableMessages(t *testing.T) {
	retiredCopy := func(msg string) bool {
		lower := strings.ToLower(msg)
		return strings.Contains(lower, "invalid url (post /api/user/checkin)") ||
			(strings.Contains(lower, "http 404") && strings.Contains(lower, "/api/user/checkin")) ||
			strings.Contains(lower, "checkin endpoint not found") ||
			strings.Contains(lower, "check-in is not supported") ||
			strings.Contains(lower, "checkin is not supported") ||
			strings.Contains(lower, "does not support checkin") ||
			strings.Contains(lower, "not support checkin")
	}

	// Every wording either list can produce, plus the wordings that decide
	// whether the gate is reached at all.
	cases := []string{
		"",
		"unexpected token < in JSON",
		"not valid json",
		"<html>challenge</html>",
		"missing new-api-user header",
		"invalid access token",
		"401 unauthorized",
		"403 forbidden",
		"not login",
		"not logged in",
		"未登录",
		"参数未提供",
		"invalid url (POST /api/user/checkin)",
		"HTTP 404: /api/user/checkin not found",
		"checkin endpoint not found",
		"check-in is not supported",
		"checkin is not supported",
		"does not support checkin",
		"not support checkin",
		"签到功能未启用",
		"normal error",
	}
	compared := 0
	for _, msg := range cases {
		reachable := msg == "" || shouldFallbackToCookieCheckin(msg)
		got, want := IsCheckinEndpointAbsent(msg), retiredCopy(msg)
		if reachable {
			compared++
			if got != want {
				t.Errorf("reachable message %q: gate = %v, retired copy = %v (behavior change)", msg, got, want)
			}
			continue
		}
		if got != want {
			t.Logf("unreachable divergence, retired with the copy: %q (gate=%v copy=%v)", msg, got, want)
		}
	}
	if compared < 2 {
		t.Fatalf("only %d reachable cases were compared; the equivalence claim would be vacuous", compared)
	}

	// The five patterns the retired copy carried but the gate never saw. If any
	// of them ever becomes reachable, the shared predicate has to cover it.
	for _, msg := range []string{
		"checkin endpoint not found",
		"check-in is not supported",
		"checkin is not supported",
		"does not support checkin",
		"not support checkin",
	} {
		if shouldFallbackToCookieCheckin(msg) {
			t.Errorf("%q now reaches the cookie-ladder gate; re-examine the retirement of its pattern", msg)
		}
		if !IsUnsupportedCheckinMessage(msg) {
			t.Errorf("%q is no longer recognized as unsupported anywhere; coverage was lost, not folded", msg)
		}
	}
}

// --- IsCookieSessionFailureMessage ---

func TestIsCookieSessionFailureMessage(t *testing.T) {
	tests := []struct {
		msg    string
		isFail bool
	}{
		{"access token invalid", true},
		{"unauthorized", true},
		{"new-api-user header missing", true},
		{"user id required", true},
		{"invalid token", true},
		{"token expired", true},
		{"未登录", true},
		{"not login", true},
		{"success", false},
		{"checkin completed", false},
		// Non-auth residual: bare "expired" alone must not look like cookie session failure
		// when the class is billing/model (R0).
		{"No payment method. Add a payment method here: https://example.com/billing", false},
		{"Model foo is not supported for format openai", false},
		{"rate limit exceeded", false},
	}
	for _, tt := range tests {
		if got := isCookieSessionFailureMessage(tt.msg); got != tt.isFail {
			t.Errorf("isCookieSessionFailureMessage(%q) = %v, want %v", tt.msg, got, tt.isFail)
		}
	}
}

// --- BuildUserIDProbeCandidates ---

func TestBuildUserIDProbeCandidates(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}

	// Plain token - should include hardcoded list
	candidates := n.buildUserIDProbeCandidates("plain-token")
	// Hardcoded list: 1,2,3,4,5,6,7,8,9,10,15,20,50,100,8899,11494 = 16 items
	if len(candidates) < 16 {
		t.Errorf("expected at least 16 candidates (hardcoded list), got %d: %v", len(candidates), candidates)
	}

	// Verify hardcoded IDs are present
	expectedIDs := map[int]bool{
		1: true, 2: true, 3: true, 4: true, 5: true,
		6: true, 7: true, 8: true, 9: true, 10: true,
		15: true, 20: true, 50: true, 100: true, 8899: true, 11494: true,
	}
	for _, c := range candidates {
		if expectedIDs[c] {
			delete(expectedIDs, c)
		}
	}
	if len(expectedIDs) > 0 {
		t.Errorf("missing expected IDs: %v", expectedIDs)
	}
}

func TestExtractLikelyUserIDs_EmptyToken(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
	ids := n.extractLikelyUserIDs("")
	if len(ids) != 0 {
		t.Errorf("empty token should return empty: %v", ids)
	}
}

func TestNewApiAdapter_VerifyToken_NewApiUserHeader401(t *testing.T) {
	// Site requires the New-Api-User header: Bearer-only request returns
	// HTTP 401 with "New-Api-User header not provided"; retry with the header
	// succeeds. VerifyToken must fall back to the header path on 401.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("New-Api-User") != "17243" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Unauthorized, New-Api-User header not provided","success":false}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"id":17243,"username":"demo","display_name":"Demo","quota":100.5}}`)
	}))
	defer srv.Close()

	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
	uid := 17243
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := n.VerifyToken(ctx, srv.URL, "osH0EmYw+1yQUJ0DdL3X8deT8E8X0Q==", &uid, nil)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if result.TokenType != "session" {
		t.Fatalf("TokenType = %q, want session", result.TokenType)
	}
	if result.UserInfo == nil || result.UserInfo.Username != "demo" {
		t.Fatalf("UserInfo = %+v, want username demo", result.UserInfo)
	}
}

func TestNewApiAdapter_VerifyToken_NewApiUserHeader200Message(t *testing.T) {
	// Site variant that returns HTTP 200 with the same message (legacy
	// behavior) must also reach the header retry path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("New-Api-User") != "42" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"message":"Unauthorized, New-Api-User header not provided","success":false}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"id":42,"username":"user42","quota":1.0}}`)
	}))
	defer srv.Close()

	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
	uid := 42
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := n.VerifyToken(ctx, srv.URL, "some-token", &uid, nil)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if result.TokenType != "session" {
		t.Fatalf("TokenType = %q, want session", result.TokenType)
	}
}
