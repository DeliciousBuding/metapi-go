package notify

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNotifyHTTPClientHasTimeout(t *testing.T) {
	if notifyHTTPClient.Timeout != notifyHTTPTimeout {
		t.Fatalf("notifyHTTPClient.Timeout = %s, want %s", notifyHTTPClient.Timeout, notifyHTTPTimeout)
	}
	if notifyHTTPClient.Timeout == 0 {
		t.Fatal("notifyHTTPClient must not use the default no-timeout behavior")
	}
	if notifyHTTPClient.CheckRedirect == nil {
		t.Fatal("notifyHTTPClient must validate redirects")
	}
	transport, ok := notifyHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("notifyHTTPClient.Transport = %T, want *http.Transport", notifyHTTPClient.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("notifyHTTPClient must not use environment proxy for user-configured notification URLs")
	}
}

func TestReadNotifyResponseBodyRejectsOversizedBody(t *testing.T) {
	body := strings.NewReader(strings.Repeat("x", notifyMaxResponseBodySize+1))
	_, err := readNotifyResponseBody(body)
	if err == nil {
		t.Fatal("expected oversized notification response body to fail")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %q, want size-limit failure", err)
	}
}

func TestNotifyRequestRejectsUnsafeTargetsBeforeDial(t *testing.T) {
	oldClient := notifyHTTPClient
	t.Cleanup(func() {
		notifyHTTPClient = oldClient
	})

	called := false
	notifyHTTPClient = &http.Client{
		Timeout: notifyHTTPTimeout,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Request:    req,
			}, nil
		}),
	}

	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1/notify", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	_, err = doNotifyRequest(req)
	if err == nil {
		t.Fatal("doNotifyRequest allowed loopback URL, want rejection")
	}
	if called {
		t.Fatal("transport was called for unsafe notification URL")
	}
}

func TestNotifyURLValidationRejectsUnsafeTargets(t *testing.T) {
	tests := []string{
		"http://localhost/notify",
		"http://localhost./notify",
		"http://127.0.0.1/notify",
		"http://[::1]/notify",
		"http://10.0.0.5/notify",
		"http://172.16.0.5/notify",
		"http://192.168.1.5/notify",
		"http://169.254.169.254/latest/meta-data",
		"http://[fe80::1]/notify",
		"http://224.0.0.1/notify",
		"http://0.0.0.0/notify",
		"http://0.1.2.3/notify", // 0.0.0.0/8 non-zero address (audit L11)
		"file:///tmp/notify",
		"https://user:pass@example.com/notify",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			parsed, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("parse test URL: %v", err)
			}
			if err := validateNotifyRequestURL(parsed); err == nil {
				t.Fatal("validateNotifyRequestURL returned nil, want rejection")
			}
		})
	}

	parsed, err := url.Parse("https://notify.example.com/hook")
	if err != nil {
		t.Fatalf("parse public URL: %v", err)
	}
	if err := validateNotifyRequestURL(parsed); err != nil {
		t.Fatalf("public notification URL rejected: %v", err)
	}
}

func TestNotifyChannelsUseBoundedHTTPClient(t *testing.T) {
	oldClient := notifyHTTPClient
	t.Cleanup(func() {
		notifyHTTPClient = oldClient
	})

	var requests []*http.Request
	notifyHTTPClient = &http.Client{
		Timeout: notifyHTTPTimeout,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests = append(requests, req)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Request:    req,
			}, nil
		}),
	}

	if err := (&BarkChannel{}).Send(&config.RuntimeSettings{
		BarkEnabled: true,
		BarkUrl:     "https://bark.example",
	}, "title", "message", "info", "time"); err != nil {
		t.Fatalf("BarkChannel.Send: %v", err)
	}
	if err := (&ServerChanChannel{}).Send(&config.RuntimeSettings{
		ServerChanEnabled: true,
		ServerChanKey:     "serverchan-key",
	}, "title", "message", "warning", "time"); err != nil {
		t.Fatalf("ServerChanChannel.Send: %v", err)
	}
	if err := (&WebhookChannel{}).Send(&config.RuntimeSettings{
		WebhookEnabled: true,
		WebhookUrl:     "https://webhook.example/hook",
	}, "title", "message", "error", "time"); err != nil {
		t.Fatalf("WebhookChannel.Send: %v", err)
	}

	if len(requests) != 3 {
		t.Fatalf("captured %d notify requests, want 3", len(requests))
	}
	if requests[0].Method != http.MethodGet {
		t.Fatalf("bark method = %s, want GET", requests[0].Method)
	}
	if requests[1].Method != http.MethodPost {
		t.Fatalf("serverchan method = %s, want POST", requests[1].Method)
	}
	if got := requests[1].Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("serverchan content-type = %q", got)
	}
	if requests[2].Method != http.MethodPost {
		t.Fatalf("webhook method = %s, want POST", requests[2].Method)
	}
	if got := requests[2].Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("webhook content-type = %q", got)
	}
}

// useTelegramTestClient swaps the package-level Telegram client so response
// handling can be exercised without a network. The production client and its
// public-only dial guard are checked by TestTelegramRejectsPrivateAPIBase.
func useTelegramTestClient(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	old := telegramHTTPClient
	telegramHTTPClient = &http.Client{
		Timeout:       time.Second,
		Transport:     transport,
		CheckRedirect: platform.RejectCrossOriginRedirect,
	}
	t.Cleanup(func() { telegramHTTPClient = old })
}

func TestTelegramRejectsOversizedResponseBody(t *testing.T) {
	useTelegramTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", notifyMaxResponseBodySize+1))),
			Request:    req,
		}, nil
	}))

	err := (&TelegramChannel{}).Send(&config.RuntimeSettings{
		TelegramEnabled:    true,
		TelegramBotToken:   "bot-token",
		TelegramChatId:     "123",
		TelegramApiBaseUrl: "https://api.telegram.org",
	}, "title", "message", "info", "time")
	if err == nil {
		t.Fatal("expected oversized telegram response body to fail")
	}
	if !strings.Contains(err.Error(), "telegram response read failed") {
		t.Fatalf("error = %q, want telegram response read failure", err)
	}
}

func TestTelegramRejectsCrossOriginRedirect(t *testing.T) {
	useTelegramTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Location", "https://evil.example/landing")
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	}))

	err := (&TelegramChannel{}).Send(&config.RuntimeSettings{
		TelegramEnabled:    true,
		TelegramBotToken:   "bot-token",
		TelegramChatId:     "123",
		TelegramApiBaseUrl: "https://api.telegram.org",
	}, "title", "message", "info", "time")
	if err == nil {
		t.Fatal("expected cross-origin telegram redirect to fail")
	}
	if !strings.Contains(err.Error(), "telegram request failed") {
		t.Fatalf("error = %q, want telegram request failure", err)
	}
	if !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("error = %q, want cross-origin", err)
	}
}

// TestTelegramRejectsPrivateAPIBase is the M2 regression: the API base URL is
// session-writable, so a private/metadata target must be rejected before any
// connection attempt.
func TestTelegramRejectsPrivateAPIBase(t *testing.T) {
	cases := []string{
		"http://127.0.0.1:8080",
		"http://169.254.169.254",
		"http://[fd20:ce::254]",
		"http://localhost",
	}
	for _, base := range cases {
		t.Run(base, func(t *testing.T) {
			err := (&TelegramChannel{}).Send(&config.RuntimeSettings{
				TelegramEnabled:    true,
				TelegramBotToken:   "bot-token",
				TelegramChatId:     "123",
				TelegramApiBaseUrl: base,
			}, "title", "message", "info", "time")
			if err == nil {
				t.Fatal("telegram accepted a private API base URL")
			}
			if !strings.Contains(err.Error(), "request target rejected") {
				t.Fatalf("error = %q, want request target rejection", err)
			}
		})
	}
}

// TestTelegramDoesNotEchoUpstreamResponseBody pins the second half of M2: a
// repointed API base must not reflect internal response content into the
// admin-visible error.
func TestTelegramDoesNotEchoUpstreamResponseBody(t *testing.T) {
	const secret = "INTERNAL-SECRET-VALUE-42"
	useTelegramTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"description":"` + secret + `"}`)),
			Request:    req,
		}, nil
	}))

	err := (&TelegramChannel{}).Send(&config.RuntimeSettings{
		TelegramEnabled:    true,
		TelegramBotToken:   "bot-token",
		TelegramChatId:     "123",
		TelegramApiBaseUrl: "https://api.telegram.org",
	}, "title", "message", "info", "time")
	if err == nil {
		t.Fatal("expected telegram API rejection to fail")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error echoes upstream response body: %q", err)
	}
}
