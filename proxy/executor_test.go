package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoWithObservedFirstByteReturnsTransportErrors(t *testing.T) {
	wantErr := errors.New("connection refused")
	executor := &RuntimeExecutor{client: &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, wantErr
		}),
	}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := executor.DoWithObservedFirstByte(req.Context(), req, 1000)
	if resp != nil {
		resp.Body.Close()
		t.Fatal("transport failure returned a response")
	}
	if !errors.Is(err, wantErr) || IsObservedFirstByteTimeoutError(err) {
		t.Fatalf("error = %v, want original transport error, not a header timeout", err)
	}
}

func TestFirstByteTimeoutMsSecToMs(t *testing.T) {
	tests := []struct {
		sec  int
		want int64
	}{
		{0, 0},
		{-1, 0},
		{1, 1000},
		{5, 5000},
		{90, 90000},
	}
	for _, tt := range tests {
		if got := FirstByteTimeoutMs(tt.sec); got != tt.want {
			t.Errorf("FirstByteTimeoutMs(%d) = %d, want %d", tt.sec, got, tt.want)
		}
	}
}

func TestDoWithObservedFirstByteTimeoutError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	executor := NewRuntimeExecutor(time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := executor.DoWithObservedFirstByte(context.Background(), req, 5)
	if resp != nil {
		resp.Body.Close()
		t.Fatalf("expected nil response on first-byte timeout")
	}
	if !IsObservedFirstByteTimeoutError(err) {
		t.Fatalf("err = %v, want ErrObservedFirstByteTimeout", err)
	}
}

func TestDoWithObservedFirstByteDoesNotCancelBodyAfterHeaders(t *testing.T) {
	// The discriminator is window < body delay: if the first-byte timer kept
	// firing after headers arrived, the delayed body read would fail. Both
	// values stay generous so loopback scheduling latency under -race or a
	// loaded CI machine cannot flake the header-arrival leg.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(1500 * time.Millisecond)
		_, _ = w.Write([]byte("body-after-headers"))
	}))
	t.Cleanup(upstream.Close)

	executor := NewRuntimeExecutor(5 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// Headers arrive immediately; the body follows after the window expires.
	resp, err := executor.DoWithObservedFirstByte(context.Background(), req, 1000)
	if err != nil {
		t.Fatalf("DoWithObservedFirstByte: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "body-after-headers" {
		t.Fatalf("body = %q, want body-after-headers", body)
	}
}

func TestRuntimeExecutorBufferedResponseLimit(t *testing.T) {
	prev, prevRt := config.GetSafe(), config.RuntimeSafe()
	t.Cleanup(func() { config.Set(prev); config.SetRuntime(prevRt) })
	cfg, rt := config.Load(map[string]string{"PROXY_MAX_BUFFERED_RESPONSE_BYTES": "8"})
	config.Set(cfg)
	config.SetRuntime(rt)

	for _, stream := range []bool{false, true} {
		for _, size := range []int{8, 9} {
			t.Run(fmt.Sprintf("stream=%t/size=%d", stream, size), func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, strings.Repeat("x", size))
				}))
				t.Cleanup(upstream.Close)
				executor := NewRuntimeExecutor(time.Second)
				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
				dispatch := executor.DoWithObservedFirstByte
				if stream {
					dispatch = executor.DoStreamWithObservedFirstByte
				}
				resp, err := dispatch(req.Context(), req, 1000)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				body, err := ReadBufferedResponseBody(resp.Body)
				if size == 8 {
					if err != nil || string(body) != strings.Repeat("x", size) {
						t.Fatalf("body = %q, error = %v", body, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "response body exceeded") {
					t.Fatalf("error = %v, want response limit rejection", err)
				}
			})
		}
	}
}

func TestRuntimeExecutorRejectsCrossOriginRedirect(t *testing.T) {
	// SSRF surface: 302 to a different host (e.g. metadata/loopback) must not be followed.
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ssrf-payload")
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/metadata", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	executor := NewRuntimeExecutor(time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, source.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := executor.DoWithObservedFirstByte(req.Context(), req, 1000)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("executor allowed cross-origin redirect")
	}
	if !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("error = %v, want cross-origin redirect rejection", err)
	}
	if targetCalled {
		t.Fatal("cross-origin redirect target was called (SSRF)")
	}
}

func TestRuntimeExecutorAllowsSameHostRedirect(t *testing.T) {
	// Mirror platform policy: same-host redirects remain allowed.
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/landing", http.StatusFound)
	})
	mux.HandleFunc("/landing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "same-host-ok")
	})
	upstream := httptest.NewServer(mux)
	t.Cleanup(upstream.Close)

	executor := NewRuntimeExecutor(time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := executor.DoWithObservedFirstByte(req.Context(), req, 1000)
	if err != nil {
		t.Fatalf("same-host redirect: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := ReadBufferedResponseBody(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "same-host-ok" {
		t.Fatalf("body = %q, want same-host-ok", body)
	}
}

func TestRejectCrossOriginRedirectPolicy(t *testing.T) {
	viaHTTPS, err := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)
	if err != nil {
		t.Fatalf("NewRequest via: %v", err)
	}
	toHTTP, err := http.NewRequest(http.MethodGet, "http://api.example.com/v1", nil)
	if err != nil {
		t.Fatalf("NewRequest to: %v", err)
	}
	if err = rejectCrossOriginRedirect(toHTTP, []*http.Request{viaHTTPS}); err == nil {
		t.Fatal("expected https→http redirect rejection")
	}

	toOtherHost, err := http.NewRequest(http.MethodGet, "https://169.254.169.254/latest/meta-data/", nil)
	if err != nil {
		t.Fatalf("NewRequest metadata: %v", err)
	}
	if err = rejectCrossOriginRedirect(toOtherHost, []*http.Request{viaHTTPS}); err == nil {
		t.Fatal("expected cross-origin metadata redirect rejection")
	}
	if !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("error = %v, want cross-origin", err)
	}

	sameHost, err := http.NewRequest(http.MethodGet, "https://api.example.com/v2", nil)
	if err != nil {
		t.Fatalf("NewRequest same: %v", err)
	}
	if err = rejectCrossOriginRedirect(sameHost, []*http.Request{viaHTTPS}); err != nil {
		t.Fatalf("same-host https redirect should be allowed: %v", err)
	}
}
