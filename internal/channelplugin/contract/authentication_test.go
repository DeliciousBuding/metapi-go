package contract

import "testing"

func TestAuthenticationContractRestrictsHeaderAndClockAuthority(t *testing.T) {
	valid := Authentication{Type: "jwt_hs256", Header: "Authorization", Prefix: "Bearer ", JWT: &JWTAuthentication{KeySeparator: "|", IssuerClaim: "iss", TTLSeconds: 1800, TimeUnit: "seconds"}}
	if valid.Validate() != nil {
		t.Fatal("valid host authentication rejected")
	}
	for _, mutate := range []func(*Authentication){
		func(a *Authentication) { a.Header = "Host" },
		func(a *Authentication) { a.Header = "Cookie" },
		func(a *Authentication) { a.Header = "Proxy-Authorization" },
		func(a *Authentication) { a.Header = "Sec-WebSocket-Key" },
		func(a *Authentication) { a.Header = "X-Forwarded-For" },
		func(a *Authentication) { a.Header = "X-Key\n" },
		func(a *Authentication) { a.Prefix = "Bearer\r\n" },
		func(a *Authentication) { a.Type = "none" },
		func(a *Authentication) { a.JWT.TTLSeconds = 1 },
		func(a *Authentication) { a.JWT.TTLSeconds = 86401 },
		func(a *Authentication) { a.JWT.TimeUnit = "ticks" },
		func(a *Authentication) { a.JWT.IssuerClaim = "exp" },
		func(a *Authentication) { a.JWT.IssuedAtClaim = "iss" },
		func(a *Authentication) { a.JWT.SignType = "unsafe" },
	} {
		candidate := valid
		jwt := *valid.JWT
		candidate.JWT = &jwt
		mutate(&candidate)
		if candidate.Validate() == nil {
			t.Fatal("invalid authentication accepted")
		}
	}
}

func TestAuthenticationContractCannotReplaceScopedSessionAndProtocolHeaders(t *testing.T) {
	for _, header := range []string{
		"Session-Id", "Conversation_Id", "Conversation-Id", "Thread-Id",
		"X-Codex-Window-Id", "X-Codex-Turn-Metadata", "X-Claude-Code-Session-Id",
		"Chatgpt-Account-Id", "Anthropic-Version", "Anthropic-Beta", "OpenAI-Beta",
		"OpenAI-Organization", "OpenAI-Project", "X-Request-Id", "X-Client-Request-Id",
		"Request-Id", "Traceparent", "Tracestate", "Baggage",
	} {
		if (Authentication{Type: "api_key", Header: header}).Validate() == nil {
			t.Errorf("credential presentation can replace a scoped identity/protocol header: %s", header)
		}
	}
}
