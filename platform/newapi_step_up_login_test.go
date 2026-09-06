package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	stepUpUsername = "test-user"
	stepUpPassword = "  test-password-with-spaces!  "
	stepUpJWT      = "test-short-lived-dashboard-jwt"
	stepUpSID      = "test-session-id"
	stepUpCookie   = "new_api_refresh=test-session-id.test-refresh-secret"
	stepUpProof    = "test-single-use-security-proof"
	stepUpPAT      = "test-durable-dashboard-pat"
)

type stepUpResponse struct {
	status int
	body   string
}

// The fixture follows QuantumNous/new-api at 0c76e4dae77a279e015329b7478e6f02d6b62edd:
// router/api-router.go, controller/{access_token,secure_verification,auth_session}.go
// and service/security_verification.go. A proof is session- and scope-bound,
// consumed once, and is NOT a cookie or a durable dashboard credential.
func newAPIStepUpServer(t *testing.T, overrides map[string]stepUpResponse) (*httptest.Server, func() []string) {
	t.Helper()
	responses := map[string]stepUpResponse{
		"login":     {200, `{"success":true,"data":{"access_token":"` + stepUpJWT + `","access_expires_at":1893456000,"session":{"sid":"` + stepUpSID + `"}}}`},
		"pat":       {403, `{"success":false,"code":"SECURITY_PROOF_REQUIRED","message":"需要安全验证"}`},
		"methods":   {200, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"password","available":true}],"oauth_providers":[],"password_encryption_enabled":false}}`},
		"verify":    {200, stepUpProofBody(stepUpProof, "password", "access_token.generate", 1893456000)},
		"pat-retry": {200, `{"success":true,"message":"","data":"` + stepUpPAT + `"}`},
		"logout":    {200, `{"success":true,"message":"","data":{"revoked_sid":"` + stepUpSID + `","cookie_cleared":true}}`},
	}
	for step, response := range overrides {
		responses[step] = response
	}
	var mu sync.Mutex
	var calls []string
	patCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var step string
		switch r.Method + " " + r.URL.Path {
		case "POST /api/user/login":
			step = "login"
		case "GET /api/user/token":
			mu.Lock()
			patCalls++
			count := patCalls
			mu.Unlock()
			step = "pat"
			if count == 2 {
				step = "pat-retry"
			} else if count > 2 {
				t.Error("PAT was retried more than once")
				step = "unexpected-pat-retry"
			}
		case "GET /api/verify/methods":
			step = "methods"
		case "POST /api/verify":
			step = "verify"
		case "POST /api/user/auth/logout":
			step = "logout"
		default:
			step = "unexpected: " + r.Method + " " + r.URL.Path
			t.Error("unexpected upstream request")
		}
		mu.Lock()
		calls = append(calls, step)
		mu.Unlock()

		if step == "login" || step == "verify" {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error("invalid authentication request JSON")
			}
			want := map[string]string{"username": stepUpUsername, "password": stepUpPassword}
			if step == "verify" {
				want = map[string]string{"method": "password", "scope": "access_token.generate", "password": stepUpPassword}
			}
			if !reflect.DeepEqual(body, want) {
				t.Error("authentication request did not use exactly the original password and official fields")
			}
		} else if r.ContentLength > 0 {
			t.Error("password or other body unexpectedly sent outside login/verification")
		}
		if step == "methods" {
			if r.URL.RawQuery != "scope=access_token.generate" {
				t.Error("verification methods requested for the wrong scope")
			}
		} else if r.URL.RawQuery != "" {
			t.Error("unexpected authentication query parameters")
		}
		if step != "login" {
			if r.Header.Get("Authorization") != "Bearer "+stepUpJWT || r.Header.Get("X-Auth-Session") != stepUpSID {
				t.Error("authentication did not retain the original login session")
			}
			if r.Header.Get("New-Api-User") != "17" {
				t.Error("authentication changed the requested upstream user")
			}
		}
		if step == "pat-retry" {
			if r.Header.Get("X-Security-Proof") != stepUpProof {
				t.Error("PAT retry did not carry the issued security proof")
			}
		} else if r.Header.Get("X-Security-Proof") != "" {
			t.Error("security proof sent outside the single PAT retry")
		}
		if step == "logout" {
			if r.Header.Get("Cookie") != stepUpCookie {
				t.Error("logout lost the login refresh cookie")
			}
			if r.Header.Get("Origin") != "http://"+r.Host {
				t.Error("logout must satisfy the upstream same-origin cookie guard")
			}
		} else if r.Header.Get("Cookie") != "" {
			t.Error("refresh cookie escaped its /api/user/auth path")
		}
		if step == "login" {
			w.Header().Set("Set-Cookie", stepUpCookie+"; Path=/api/user/auth; HttpOnly; SameSite=Strict")
		}
		response, ok := responses[step]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.status)
		fmt.Fprint(w, response.body)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

func stepUpProofBody(token, method, scope string, expires int64) string {
	body, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"data":    map[string]interface{}{"proof_token": token, "method": method, "scope": scope, "expires_at": expires},
	})
	return string(body)
}

func stepUpSecretError(code string) string {
	body, _ := json.Marshal(map[string]interface{}{
		"success": false, "code": code,
		"message": strings.Join([]string{stepUpUsername, stepUpPassword, stepUpJWT, stepUpSID, stepUpCookie, stepUpProof, stepUpPAT}, " | "),
	})
	return string(body)
}

func captureStepUpLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

func assertStepUpNoSecrets(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{stepUpUsername, stepUpPassword, stepUpJWT, stepUpSID, stepUpCookie, stepUpProof, stepUpPAT} {
		if strings.Contains(text, secret) {
			t.Error("authentication response or logs leaked a credential/session value")
		}
	}
}

func TestNewApiAdapter_Login_V1PasswordStepUp(t *testing.T) {
	for _, logoutFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("logoutFailure=%t", logoutFails), func(t *testing.T) {
			logs := captureStepUpLogs(t)
			overrides := map[string]stepUpResponse{}
			if logoutFails {
				overrides["logout"] = stepUpResponse{403, stepUpSecretError("AUTH_ORIGIN_FORBIDDEN")}
			}
			srv, calls := newAPIStepUpServer(t, overrides)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			userID := 17
			result, err := newApiAdapterUnderTest().Login(ctx, srv.URL, stepUpUsername, stepUpPassword, &userID, nil)
			if err != nil || result == nil || !result.Success || result.AccessToken != stepUpPAT {
				t.Fatalf("password step-up did not return the durable PAT: result=%+v err=%v", result, err)
			}
			want := []string{"login", "pat", "methods", "verify", "pat-retry", "logout"}
			if !reflect.DeepEqual(calls(), want) {
				t.Fatalf("requests = %v, want %v", calls(), want)
			}
			if logoutFails && logs.Len() == 0 {
				t.Error("best-effort logout failure must be observable")
			}
			assertStepUpNoSecrets(t, result.Message+logs.String())
		})
	}
}

func TestNewApiAdapter_Login_V1StepUpFailsClosed(t *testing.T) {
	beforeMethods := []string{"login", "pat", "logout"}
	beforeVerify := []string{"login", "pat", "methods", "logout"}
	beforeRetry := []string{"login", "pat", "methods", "verify", "logout"}
	afterRetry := []string{"login", "pat", "methods", "verify", "pat-retry", "logout"}
	cases := []struct {
		name     string
		step     string
		response stepUpResponse
		calls    []string
	}{
		{"wrong_password", "verify", stepUpResponse{200, stepUpSecretError("SECURITY_VERIFICATION_FAILED")}, beforeRetry},
		{"policy_changed_to_mfa", "verify", stepUpResponse{200, stepUpSecretError("SECURITY_PROOF_METHOD_MISMATCH")}, beforeRetry},
		{"two_factor_required", "methods", stepUpResponse{200, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"2fa","available":true}],"password_encryption_enabled":false}}`}, beforeVerify},
		{"passkey_required", "methods", stepUpResponse{200, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"passkey","available":true}],"password_encryption_enabled":false}}`}, beforeVerify},
		{"oauth_required", "methods", stepUpResponse{200, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"oauth","available":true}],"password_encryption_enabled":false}}`}, beforeVerify},
		{"password_unavailable", "methods", stepUpResponse{200, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"password","available":false}],"password_encryption_enabled":false}}`}, beforeVerify},
		{"encrypted_password_required", "methods", stepUpResponse{200, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"password","available":true}],"password_encryption_enabled":true}}`}, beforeVerify},
		{"unknown_encryption_policy", "methods", stepUpResponse{200, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"password","available":true}]}}`}, beforeVerify},
		{"different_operation", "methods", stepUpResponse{200, `{"success":true,"data":{"scope":"access_token.revoke","methods":[{"method":"password","available":true}],"password_encryption_enabled":false}}`}, beforeVerify},
		{"methods_not_supported", "methods", stepUpResponse{404, stepUpSecretError("NOT_FOUND")}, beforeVerify},
		{"non_step_up_403", "pat", stepUpResponse{403, stepUpSecretError("SECURITY_ACTION_FORBIDDEN")}, beforeMethods},
		{"message_alone_is_not_a_challenge", "pat", stepUpResponse{403, `{"success":false,"message":"需要安全验证 SECURITY_PROOF_REQUIRED"}`}, beforeMethods},
		{"challenge_code_without_403", "pat", stepUpResponse{401, stepUpSecretError("SECURITY_PROOF_REQUIRED")}, beforeMethods},
		{"challenge_code_with_200", "pat", stepUpResponse{200, stepUpSecretError("SECURITY_PROOF_REQUIRED")}, beforeMethods},
		{"invalid_json_403", "pat", stepUpResponse{403, "not JSON: " + stepUpPassword + " " + stepUpJWT}, beforeMethods},
		{"failed_pat_envelope_with_data", "pat", stepUpResponse{200, `{"success":false,"data":"` + stepUpPAT + `"}`}, beforeMethods},
		{"proof_still_required_after_retry", "pat-retry", stepUpResponse{403, stepUpSecretError("SECURITY_PROOF_REQUIRED")}, afterRetry},
		{"pat_retry_server_error", "pat-retry", stepUpResponse{500, stepUpSecretError("AUTH_INTERNAL_ERROR")}, afterRetry},
		{"pat_must_not_be_session_jwt", "pat-retry", stepUpResponse{200, `{"success":true,"data":"` + stepUpJWT + `"}`}, afterRetry},
		{"pat_must_not_be_security_proof", "pat-retry", stepUpResponse{200, `{"success":true,"data":"` + stepUpProof + `"}`}, afterRetry},
		{"wrong_proof_scope", "verify", stepUpResponse{200, stepUpProofBody(stepUpProof, "password", "access_token.revoke", 1893456000)}, beforeRetry},
		{"wrong_proof_method", "verify", stepUpResponse{200, stepUpProofBody(stepUpProof, "2fa", "access_token.generate", 1893456000)}, beforeRetry},
		{"expired_proof", "verify", stepUpResponse{200, stepUpProofBody(stepUpProof, "password", "access_token.generate", 1)}, beforeRetry},
		{"missing_proof", "verify", stepUpResponse{200, stepUpProofBody("", "password", "access_token.generate", 1893456000)}, beforeRetry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureStepUpLogs(t)
			srv, calls := newAPIStepUpServer(t, map[string]stepUpResponse{tc.step: tc.response})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			userID := 17
			result, err := newApiAdapterUnderTest().Login(ctx, srv.URL, stepUpUsername, stepUpPassword, &userID, nil)
			if err != nil || result == nil {
				t.Fatalf("Login should return an explicit rejected result, err=%v", err)
			}
			if result.Success || result.AccessToken != "" {
				t.Fatal("failed verification returned a credential")
			}
			if !strings.Contains(result.Message, "manually") || !strings.Contains(result.Message, "PAT") {
				t.Errorf("missing manual PAT recovery instruction: %q", result.Message)
			}
			if !reflect.DeepEqual(calls(), tc.calls) {
				t.Fatalf("requests = %v, want %v", calls(), tc.calls)
			}
			assertStepUpNoSecrets(t, result.Message+logs.String())
		})
	}
}

func TestNewApiAdapter_Login_MFAChallengeRequiresManualPAT(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/user/login" {
			t.Error("MFA login must not attempt a bypass or PAT request")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"require_verification":true,"flow_token":"test-mfa-flow-token","expires_at":1893456000,"methods":[{"method":"2fa","available":true}]}}`)
	}))
	t.Cleanup(srv.Close)
	result, err := newApiAdapterUnderTest().Login(context.Background(), srv.URL, stepUpUsername, stepUpPassword, nil, nil)
	if err != nil || result == nil || result.Success || result.AccessToken != "" || !strings.Contains(result.Message, "MFA") || !strings.Contains(result.Message, "manually") {
		t.Fatalf("expected explicit manual PAT recovery for MFA: result=%+v err=%v", result, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("requests = %d, want only the login", calls.Load())
	}
	assertStepUpNoSecrets(t, result.Message)
	if strings.Contains(result.Message, "test-mfa-flow-token") {
		t.Error("MFA flow token leaked")
	}
}
