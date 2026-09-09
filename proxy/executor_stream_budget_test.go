package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
)

func TestRuntimeExecutorStreamBudgetOutlivesLegacyTransportLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		executor := NewRuntimeExecutor(time.Second)
		transport := executor.streamClient.Transport.(*http.Transport)
		var requestContext context.Context
		// Model only the header wait from the real transport configuration so
		// restoring its old 30s cap fails without making the test sleep 30s.
		executor.streamClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestContext = req.Context()
			ready := time.NewTimer(35 * time.Second)
			defer ready.Stop()
			var headerLimit <-chan time.Time
			if transport.ResponseHeaderTimeout > 0 {
				timer := time.NewTimer(transport.ResponseHeaderTimeout)
				defer timer.Stop()
				headerLimit = timer.C
			}
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-headerLimit:
				return nil, errors.New("net/http: timeout awaiting response headers")
			case <-ready.C:
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: alive\n\n"))}, nil
			}
		})
		req, err := http.NewRequest(http.MethodGet, "http://upstream.invalid", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := executor.DoStreamWithObservedFirstByte(req.Context(), req, 60000)
		if err != nil {
			t.Fatalf("35s header wait must fit explicit 60s budget: %v", err)
		}
		time.Sleep(2 * time.Minute)
		if requestContext.Err() != nil {
			t.Fatalf("header or whole-request timeout killed body: %v", requestContext.Err())
		}
		if body, err := io.ReadAll(resp.Body); err != nil || string(body) != "data: alive\n\n" {
			t.Fatalf("body=%q error=%v", body, err)
		}
		resp.Body.Close()
		if requestContext.Err() != context.Canceled {
			t.Fatal("Body.Close did not release request context")
		}
	})
}

// Exercise the executor, explicit platform HTTP proxy, and the client used by
// the fallback branch through their public budget APIs, not one test client.
func streamBudgetDispatcher(t *testing.T, name, serverURL string) (string, func(*http.Request, int64) (*http.Response, error)) {
	t.Helper()
	switch name {
	case "executor":
		executor := NewRuntimeExecutor(50 * time.Millisecond)
		t.Cleanup(executor.client.CloseIdleConnections)
		t.Cleanup(executor.streamClient.CloseIdleConnections)
		return serverURL, func(req *http.Request, ms int64) (*http.Response, error) {
			return executor.DoStreamWithObservedFirstByte(req.Context(), req, ms)
		}
	case "platform-proxy":
		cfg := &platform.ProxyConfig{ProxyURL: serverURL}
		return "http://stream-budget.invalid/events", func(req *http.Request, ms int64) (*http.Response, error) {
			return platform.DoWithProxyStreamBudget(req.Context(), req, cfg, ms)
		}
	case "fallback-client":
		client := &http.Client{Transport: NewStreamTransport(), CheckRedirect: platform.RejectCrossOriginRedirect}
		t.Cleanup(client.CloseIdleConnections)
		return serverURL, func(req *http.Request, ms int64) (*http.Response, error) {
			return httpclient.DoWithHeaderBudget(req, ms, client.Do)
		}
	default:
		t.Fatalf("unknown dispatch path %q", name)
		return "", nil
	}
}

func TestStreamDispatchBudgetKeepsBodyAliveAcrossClients(t *testing.T) {
	for _, name := range []string{"executor", "platform-proxy", "fallback-client"} {
		t.Run(name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				time.Sleep(75 * time.Millisecond)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: first\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-time.After(700 * time.Millisecond):
					_, _ = io.WriteString(w, "data: second\n\n")
				case <-req.Context().Done():
				}
			}))
			t.Cleanup(upstream.Close)
			target, dispatch := streamBudgetDispatcher(t, name, upstream.URL)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := dispatch(req, 500)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || string(body) != "data: first\n\ndata: second\n\n" {
				t.Fatalf("body after header budget: body=%q error=%v", body, err)
			}
		})
	}
}

func TestStreamDispatchBudgetTimesOutAcrossClients(t *testing.T) {
	for _, name := range []string{"executor", "platform-proxy", "fallback-client"} {
		t.Run(name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
				<-req.Context().Done()
			}))
			t.Cleanup(upstream.Close)
			target, dispatch := streamBudgetDispatcher(t, name, upstream.URL)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := dispatch(req, 100)
			if resp != nil || !IsObservedFirstByteTimeoutError(err) {
				t.Fatalf("response=%v error=%v, want first-byte timeout", resp, err)
			}
		})
	}
}

func TestStreamDispatchBudgetCloseReleasesAcrossClients(t *testing.T) {
	for _, name := range []string{"executor", "platform-proxy", "fallback-client"} {
		t.Run(name, func(t *testing.T) {
			released := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-req.Context().Done()
				close(released)
			}))
			t.Cleanup(upstream.Close)
			target, dispatch := streamBudgetDispatcher(t, name, upstream.URL)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := dispatch(req, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-released:
			case <-time.After(3 * time.Second):
				t.Fatal("Body.Close did not release the upstream request")
			}
			if req.Context().Err() != nil {
				t.Fatal("Body.Close cancelled the caller context")
			}
		})
	}
}

func TestRuntimeExecutorBufferedCeilingStillCoversBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-req.Context().Done()
	}))
	t.Cleanup(upstream.Close)
	executor := NewRuntimeExecutor(100 * time.Millisecond)
	t.Cleanup(executor.client.CloseIdleConnections)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := executor.DoWithObservedFirstByte(req.Context(), req, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("buffered body error=%v, want whole-request deadline", err)
	}
}
