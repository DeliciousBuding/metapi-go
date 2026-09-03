package platform

import (
	"strings"
	"testing"
)

func TestClassifyUpstreamError_Matrix(t *testing.T) {
	cases := []struct {
		name       string
		httpStatus int
		message    string
		wantClass  UpstreamErrorClass
		wantMark   bool
	}{
		// expired / mark
		{
			name:       "jwt expired marks",
			httpStatus: 0,
			message:    "jwt expired",
			wantClass:  ClassExpired,
			wantMark:   true,
		},
		{
			name:       "token expired marks",
			httpStatus: 0,
			message:    "token expired",
			wantClass:  ClassExpired,
			wantMark:   true,
		},
		{
			name:       "invalid access token marks",
			httpStatus: 0,
			message:    "invalid access token",
			wantClass:  ClassExpired,
			wantMark:   true,
		},
		{
			name:       "chinese token expired marks",
			httpStatus: 0,
			message:    "令牌已过期",
			wantClass:  ClassExpired,
			wantMark:   true,
		},
		{
			name:       "bare 401 empty body is auth not mark",
			httpStatus: 401,
			message:    "",
			wantClass:  ClassAuth,
			wantMark:   false,
		},
		{
			name:       "HTTP 401 Unauthorized is auth not mark",
			httpStatus: 0,
			message:    "HTTP 401 Unauthorized",
			wantClass:  ClassAuth,
			wantMark:   false,
		},
		{
			name:       "401 + jwt expired still marks",
			httpStatus: 401,
			message:    "jwt expired",
			wantClass:  ClassExpired,
			wantMark:   true,
		},

		// validation — must NOT mark
		{
			name:       "invalid_argument input token limit is validation",
			httpStatus: 400,
			message:    "Error code: 400 - {'error': {'code': 'invalid_argument', 'message': 'input token limit is 202752', 'type': 'invalid_request_error'}}",
			wantClass:  ClassValidation,
			wantMark:   false,
		},
		{
			name:       "401 body that is only validation must not mark",
			httpStatus: 401,
			message:    "invalid_request_error: max_tokens is too large",
			wantClass:  ClassValidation,
			wantMark:   false,
		},
		{
			name:       "dispatch denied is validation",
			httpStatus: 403,
			message:    "does not allow /v1/chat/completions dispatch",
			wantClass:  ClassValidation,
			wantMark:   false,
		},

		// model — must NOT mark
		{
			name:       "401 model not supported is capability failure",
			httpStatus: 401,
			message:    "Model minimax-m3-free is not supported for format openai",
			wantClass:  ClassModel,
			wantMark:   false,
		},
		{
			name:       "message with HTTP 401 model unsupported is not token expiry",
			httpStatus: 0,
			message:    "HTTP 401 - Model gemini-3.1-pro-preview is not supported",
			wantClass:  ClassModel,
			wantMark:   false,
		},
		{
			name:       "chinese model unsupported is not token expiry",
			httpStatus: 400,
			message:    "当前API不支持所选模型",
			wantClass:  ClassModel,
			wantMark:   false,
		},

		// billing — must NOT mark
		{
			name:       "401 billing failure is not token expiry",
			httpStatus: 401,
			message:    "No payment method. Add a payment method here: https://example.com/billing",
			wantClass:  ClassBilling,
			wantMark:   false,
		},
		{
			name:       "insufficient_quota is billing not expired",
			httpStatus: 429,
			message:    "You exceeded your current quota, please check your plan and billing details.",
			wantClass:  ClassBilling,
			wantMark:   false,
		},
		{
			name:       "chinese balance insufficient is billing",
			httpStatus: 403,
			message:    "账户余额不足，请充值",
			wantClass:  ClassBilling,
			wantMark:   false,
		},

		// transient — must NOT mark
		{
			name:       "rate limit is transient",
			httpStatus: 429,
			message:    "rate limit exceeded",
			wantClass:  ClassTransient,
			wantMark:   false,
		},
		{
			name:       "timeout is transient",
			httpStatus: 0,
			message:    "request timed out",
			wantClass:  ClassTransient,
			wantMark:   false,
		},
		{
			name:       "5xx is transient",
			httpStatus: 502,
			message:    "bad gateway",
			wantClass:  ClassTransient,
			wantMark:   false,
		},
		{
			name:       "cloudflare challenge is transient not expired",
			httpStatus: 403,
			message:    "Cloudflare challenge required",
			wantClass:  ClassTransient,
			wantMark:   false,
		},

		// auth residual / unknown — must NOT mark
		{
			name:       "newapi missing access token is auth not expired mark",
			httpStatus: 401,
			message:    "未登录且未提供 access token",
			wantClass:  ClassAuth,
			wantMark:   false,
		},
		{
			name:       "opaque 401 body without auth signal does not mark",
			httpStatus: 401,
			message:    "upstream rejected the request",
			wantClass:  ClassUnknown,
			wantMark:   false,
		},
		{
			name:       "input token alone is not credential expiry",
			httpStatus: 0,
			message:    "input token count too high for model",
			wantClass:  ClassUnknown,
			wantMark:   false,
		},
		{
			name:       "connection timeout unknown/transient path",
			httpStatus: 0,
			message:    "connection timeout",
			wantClass:  ClassTransient,
			wantMark:   false,
		},
		{
			name:       "empty message without status is unknown",
			httpStatus: 0,
			message:    "",
			wantClass:  ClassUnknown,
			wantMark:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotClass := ClassifyUpstreamError(tc.httpStatus, tc.message)
			if gotClass != tc.wantClass {
				t.Fatalf("ClassifyUpstreamError(%d, %q) = %q, want %q",
					tc.httpStatus, tc.message, gotClass, tc.wantClass)
			}
			gotMark := ShouldMarkAccountExpired(tc.httpStatus, tc.message)
			if gotMark != tc.wantMark {
				t.Fatalf("ShouldMarkAccountExpired(%d, %q) = %v, want %v (class=%s)",
					tc.httpStatus, tc.message, gotMark, tc.wantMark, gotClass)
			}
			// IsTokenExpiredError is the historical mark/relogin gate; keep aligned with mark.
			if got := IsTokenExpiredError(tc.httpStatus, tc.message); got != tc.wantMark {
				t.Fatalf("IsTokenExpiredError(%d, %q) = %v, want %v",
					tc.httpStatus, tc.message, got, tc.wantMark)
			}
		})
	}
}

func TestIsTokenExpiredError_NonAuthUpstreamNeverMarks(t *testing.T) {
	// Table focused on the false-positive guard: non-auth upstream errors
	// must never be treated as token expiry for accounts.status='expired'.
	cases := []struct {
		name       string
		httpStatus int
		message    string
	}{
		{"validation invalid_argument", 400, "invalid_argument: input token limit is 202752"},
		{"validation invalid_request_error", 400, "type: invalid_request_error"},
		{"validation context length", 400, "This model's maximum context length is 128000 tokens"},
		{"model unsupported openai format", 401, "Model foo is not supported for format openai"},
		{"model unsupported generic", 401, "HTTP 401 - Model bar is not supported"},
		{"model not found", 404, "model_not_found: no such model"},
		{"billing payment method", 401, "No payment method. Add a payment method here: https://example.com/billing"},
		{"billing insufficient quota", 429, "insufficient_quota: You exceeded your current quota"},
		{"billing chinese balance", 403, "余额不足"},
		{"rate limit", 429, "Rate limit reached for requests"},
		{"too many requests", 429, "too many requests"},
		{"dispatch denied", 403, "site does not allow /v1/chat/completions dispatch"},
		{"dispatch denied phrase", 403, "A dispatch denied error occurred"},
		{"newapi missing access token", 401, "未登录且未提供 access token"},
		{"cloudflare", 403, "cf challenge detected"},
		{"timeout", 0, "request timed out"},
		{"5xx", 502, "bad gateway from upstream"},
		{"opaque 401", 401, "upstream rejected the request"},
			{"bare 401 empty", 401, ""},
			{"HTTP 401 Unauthorized", 0, "HTTP 401 Unauthorized"},
			{"Unauthorized only", 401, "Unauthorized"},
		{"bare token word without auth", 0, "input token encoding failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if IsTokenExpiredError(tc.httpStatus, tc.message) {
				t.Fatalf("IsTokenExpiredError(%d, %q) = true, want false", tc.httpStatus, tc.message)
			}
			if ShouldMarkAccountExpired(tc.httpStatus, tc.message) {
				t.Fatalf("ShouldMarkAccountExpired(%d, %q) = true, want false", tc.httpStatus, tc.message)
			}
		})
	}
}

func TestIsTokenExpiredError_PositiveAuthSignals(t *testing.T) {
	cases := []struct {
		name       string
		httpStatus int
		message    string
	}{
		{"jwt expired", 0, "jwt expired"},
		{"token expired", 0, "token expired"},
		{"invalid access token", 0, "invalid access token"},
		{"access token is invalid", 0, "access token is invalid"},
		{"access token chinese invalid", 0, "access token无效"},
		{"访问令牌无效", 0, "访问令牌无效"},
		{"令牌已过期", 0, "令牌已过期"},
		{"401 + jwt expired", 401, "jwt expired"},
		{"401 + invalid access token", 401, "invalid access token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !IsTokenExpiredError(tc.httpStatus, tc.message) {
				t.Fatalf("IsTokenExpiredError(%d, %q) = false, want true", tc.httpStatus, tc.message)
			}
			if !ShouldMarkAccountExpired(tc.httpStatus, tc.message) {
				t.Fatalf("ShouldMarkAccountExpired(%d, %q) = false, want true", tc.httpStatus, tc.message)
			}
		})
	}
}

// TestExplainUpstreamFailure locks the operator-facing wording for an upstream
// failure: one classified, credential-safe line plus the status it was derived
// from. The first row is the message that filed #1210 — the server logged
// `HTTP 401: Token has expired` while the body said only "balance refresh
// failed". Note that ClassifyUpstreamError reads that message as ClassUnknown
// (it is too weak to auto-mark an account expired, which is correct); explaining
// it must still not fall silent, so the renderer falls back to the status.
func TestExplainUpstreamFailure(t *testing.T) {
	cases := []struct {
		name       string
		httpStatus int
		message    string
		want       string
	}{
		{"issue 1210 observed sub2api expiry", 0, "sub2api /api/v1/auth/me: HTTP 401: Token has expired", "upstream rejected the credential (HTTP 401)"},
		{"explicit invalid access token", 0, "HTTP 400: \u65e0\u6743\u8fdb\u884c\u6b64\u64cd\u4f5c\uff0caccess token \u65e0\u6548", "upstream credential expired (HTTP 400)"},
		{"jwt expired without a status", 0, "jwt expired", "upstream credential expired"},
		{"unauthorized wording", 401, "unauthorized", "upstream rejected the credential (HTTP 401)"},
		{"forbidden", 403, "forbidden", "upstream rejected the credential (HTTP 403)"},
		{"billing", 0, "insufficient_quota: exceeded your current quota", "upstream reported a billing or quota problem"},
		{"model capability", 0, "model gpt-x is not supported", "upstream does not serve the requested model"},
		{"request validation", 0, "invalid_request_error: max_tokens too large", "upstream rejected the request"},
		{"rate limited by status", 429, "slow down", "upstream rate-limited the request (HTTP 429)"},
		{"rate limited by wording", 0, "rate limit exceeded", "upstream rate-limited the request"},
		{"server error", 503, "service unavailable", "upstream errored (HTTP 503)"},
		{"timeout with no status", 0, "request: context deadline exceeded", "upstream timed out"},
		{"connection refused", 0, "request: dial tcp 127.0.0.1:1: connect: connection refused", "upstream refused the connection"},
		{"dns failure", 0, "dial tcp: lookup up.example.invalid: no such host", "upstream host could not be resolved"},
		{"tls failure", 0, "remote error: tls: bad certificate", "upstream TLS handshake failed"},
		{"missing endpoint", 404, "no such endpoint", "upstream has no such endpoint or account (HTTP 404)"},
		{"duration is not a status but the timeout is named", 0, "request timed out after 500ms", "upstream timed out"},
		{"a port is not a status", 0, "request: dial tcp 127.0.0.1:5001: connect: connection refused", "upstream refused the connection"},
		{"unnameable keeps the caller prefix", 0, "something odd happened", ""},
		{"empty message", 0, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExplainUpstreamFailure(tc.httpStatus, tc.message); got != tc.want {
				t.Fatalf("ExplainUpstreamFailure(%d, %q) = %q, want %q", tc.httpStatus, tc.message, got, tc.want)
			}
		})
	}
}

// TestExplainUpstreamFailureNeverEchoesTheUpstreamBody is the leak half of #1210.
// Classifying is the point, echoing is not: the reason ends up in an HTTP body
// and in the UI, so a credential fragment, a request id and a host that arrived
// from upstream must not travel with it.
func TestExplainUpstreamFailureNeverEchoesTheUpstreamBody(t *testing.T) {
	message := "HTTP 401: token rejected for sk-LEAKME-9f3a2b (request id req_778899) at https://up.example.invalid/api/v1/auth/me"
	reason := ExplainUpstreamFailure(0, message)
	if reason == "" {
		t.Fatalf("ExplainUpstreamFailure(%q) = \"\", want a classified reason", message)
	}
	for _, detail := range []string{"sk-LEAKME-9f3a2b", "req_778899", "up.example.invalid", "/api/v1/auth/me", "token rejected"} {
		if strings.Contains(reason, detail) {
			t.Fatalf("reason %q echoes upstream detail %q; classify, do not echo", reason, detail)
		}
	}
}
