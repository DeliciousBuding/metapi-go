package proxy

import "testing"

func TestShouldRetryProxyRequest_StatusCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "500 always retryable", status: 500, want: true},
		{name: "502 always retryable", status: 502, want: true},
		{name: "503 always retryable", status: 503, want: true},
		{name: "504 always retryable", status: 504, want: true},
		{name: "599 always retryable", status: 599, want: true},
		{name: "408 always retryable", status: 408, want: true},
		{name: "409 always retryable", status: 409, want: true},
		{name: "425 always retryable", status: 425, want: true},
		{name: "429 always retryable", status: 429, want: true},
		{name: "401 retryable (OAuth refresh)", status: 401, want: true},
		{name: "403 retryable (OAuth refresh)", status: 403, want: true},
		{name: "400 non-retryable by default", status: 400, want: false},
		{name: "404 non-retryable by default", status: 404, want: false},
		{name: "422 non-retryable by default", status: 422, want: false},
		{name: "other status non-retryable", status: 302, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRetryProxyRequest(tt.status, tt.body); got != tt.want {
				t.Errorf("ShouldRetryProxyRequest(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestShouldRetryProxyRequest_ModelUnsupportedPatterns(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "model not supported", status: 400, body: "model not supported", want: true},
		{name: "does not support the model", status: 400, body: "This endpoint does not support the model gpt-5", want: true},
		{name: "does not support model (without 'the')", status: 400, body: "OpenAI does not support model gpt-5", want: true},
		{name: "model not found", status: 400, body: "model_not_found", want: true},
		{name: "unknown model", status: 400, body: "unknown model: gpt-5", want: true},
		{name: "invalid model", status: 400, body: "invalid model specified", want: true},
		{name: "do not have access to the model", status: 400, body: "you do not have access to the model gpt-4", want: true},
		{name: "Chinese model unsupported", status: 400, body: "当前 api 不支持所选模型", want: true},
		{name: "Chinese model not supported (variant)", status: 400, body: "不支持所选模型", want: true},
		{name: "no such model", status: 400, body: "no such model", want: true},
		{name: "unknown provider for model", status: 400, body: "unknown provider for model: gpt-4", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRetryProxyRequest(tt.status, tt.body); got != tt.want {
				t.Errorf("ShouldRetryProxyRequest(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestShouldRetryProxyRequest_NonRetryableRequestPatterns(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "invalid request body blocks retry", status: 400, body: "invalid request body", want: false},
		{name: "validation error blocks retry", status: 400, body: "validation error: field x is required", want: false},
		{name: "malformed request blocks retry", status: 400, body: "malformed request", want: false},
		{name: "invalid json blocks retry", status: 400, body: "invalid json in request body", want: false},
		{name: "cannot parse blocks retry", status: 400, body: "cannot parse request", want: false},
		{name: "500 status always retryable regardless of text", status: 500, body: "invalid request body", want: true},
		{name: "non-retryable patterns take priority over UNSUPPORTED MODEL patterns", status: 400, body: "does not support the model but also validation error", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRetryProxyRequest(tt.status, tt.body); got != tt.want {
				t.Errorf("ShouldRetryProxyRequest(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestShouldRetryProxyRequest_RetryableChannelLocalPatterns(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "invalid api key", status: 400, body: "invalid api key", want: true},
		{name: "forbidden", status: 400, body: "forbidden", want: true},
		{name: "rate limit", status: 400, body: "rate limit exceeded", want: true},
		{name: "quota exceeded", status: 400, body: "quota exceeded", want: true},
		{name: "bad gateway", status: 400, body: "bad gateway error", want: true},
		{name: "gateway timeout", status: 400, body: "gateway timeout", want: true},
		{name: "gateway time-out pattern", status: 400, body: "gateway time-out", want: true},
		{name: "service unavailable", status: 400, body: "service unavailable", want: true},
		{name: "please use /v1/responses", status: 400, body: "please use /v1/responses", want: true},
		{name: "please use /v1/messages", status: 400, body: "please use /v1/messages", want: true},
		{name: "please use /v1/chat/completions", status: 400, body: "please use /v1/chat/completions", want: true},
		{name: "unsupported endpoint", status: 400, body: "unsupported endpoint", want: true},
		{name: "cpu overloaded", status: 400, body: "cpu overloaded", want: true},
		{name: "invalid access token", status: 400, body: "invalid access token", want: true},
		{name: "unsupported legacy protocol", status: 400, body: "unsupported legacy protocol", want: true},
		{name: "unrecognized request url", status: 400, body: "unrecognized request url", want: true},
		{name: "no route matched", status: 400, body: "no route matched", want: true},
		{name: "connection timed out", status: 400, body: "connection timed out", want: true},
		{name: "request timed out", status: 400, body: "request timed out", want: true},
		{name: "read timeout", status: 400, body: "read timeout", want: true},
		{name: "timed out", status: 400, body: "the operation timed out", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRetryProxyRequest(tt.status, tt.body); got != tt.want {
				t.Errorf("ShouldRetryProxyRequest(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestShouldRetryProxyRequest_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "400 with retryable channel-local text", status: 400, body: "quota exceeded on this channel", want: true},
		{name: "400 without any retryable text", status: 400, body: "some generic error", want: false},
		{name: "404 without any retryable text", status: 404, body: "not found", want: false},
		{name: "422 without any retryable text", status: 422, body: "unprocessable", want: false},
		{name: "empty error text", status: 400, body: "", want: false},
		{name: "case-insensitive matching", status: 400, body: "Model Not Supported", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRetryProxyRequest(tt.status, tt.body); got != tt.want {
				t.Errorf("ShouldRetryProxyRequest(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestShouldAbortSameSiteEndpointFallback(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "500 with retryable pattern", status: 500, body: "service unavailable", want: true},
		{name: "429 with rate limit", status: 429, body: "rate limit exceeded", want: true},
		{name: "408 with timeout", status: 408, body: "connection timed out", want: true},
		{name: "400 should not abort", status: 400, body: "rate limit exceeded", want: false},
		{name: "503 with matching pattern", status: 503, body: "bad gateway", want: true},
		{name: "502 with non-matching text does not abort", status: 502, body: "some other error", want: false},
		{name: "502 with matching pattern does abort", status: 502, body: "service unavailable", want: true},
		{name: "connection reset", status: 500, body: "connection reset by peer", want: true},
		{name: "connection refused", status: 500, body: "connection refused", want: true},
		{name: "econnreset", status: 500, body: "econnreset", want: true},
		{name: "temporarily unavailable", status: 500, body: "service temporarily unavailable", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldAbortSameSiteEndpointFallback(tt.status, tt.body); got != tt.want {
				t.Errorf("ShouldAbortSameSiteEndpointFallback(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestGetProxyMaxChannelAttempts(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 1},
		{-1, 1},
		{-100, 1},
		{1, 1},
		{3, 3},
		{10, 10},
	}

	for _, tt := range tests {
		got := GetProxyMaxChannelAttempts(tt.input)
		if got != tt.expected {
			t.Errorf("GetProxyMaxChannelAttempts(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestGetProxyMaxChannelRetries(t *testing.T) {
	tests := []struct {
		attempts int
		expected int
	}{
		{1, 0}, // 1 attempt, 0 retries
		{2, 1}, // 2 attempts, 1 retry
		{3, 2}, // 3 attempts, 2 retries
		{5, 4}, // 5 attempts, 4 retries
		{0, 0}, // 0 attempts -> 0 retries (min)
	}

	for _, tt := range tests {
		got := GetProxyMaxChannelRetries(tt.attempts)
		if got != tt.expected {
			t.Errorf("GetProxyMaxChannelRetries(%d) = %d, want %d", tt.attempts, got, tt.expected)
		}
	}
}
