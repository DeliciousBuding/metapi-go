package httpclient

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type budgetBody struct {
	io.Reader
	close func() error
}

func (b *budgetBody) Close() error { return b.close() }

type budgetRoundTrip func(*http.Request) (*http.Response, error)

func (f budgetRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestResponseHeaderBudget(t *testing.T) {
	for _, tc := range []struct {
		ms   int64
		want time.Duration
	}{
		{0, DefaultRequestCeiling}, {-1, DefaultRequestCeiling},
		{25, 25 * time.Millisecond}, {120000, 2 * time.Minute},
		{math.MaxInt64, time.Duration(math.MaxInt64)},
	} {
		if got := responseHeaderBudget(tc.ms); got != tc.want {
			t.Errorf("responseHeaderBudget(%d) = %v, want %v", tc.ms, got, tc.want)
		}
	}
	if DefaultRequestCeiling != 90*time.Second {
		t.Fatalf("default request/header ceiling = %v, want 90s", DefaultRequestCeiling)
	}
}

func TestDoWithHeaderBudgetDefaultIsFinite(t *testing.T) {
	for _, ms := range []int64{0, -1} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(http.MethodGet, "http://upstream.invalid", nil).WithContext(ctx)
			type result struct {
				resp    *http.Response
				err     error
				elapsed time.Duration
			}
			done := make(chan result, 1)
			go func() {
				started := time.Now()
				resp, err := DoWithHeaderBudget(req, ms, func(req *http.Request) (*http.Response, error) {
					<-req.Context().Done()
					return nil, req.Context().Err()
				})
				done <- result{resp, err, time.Since(started)}
			}()
			synctest.Wait()
			time.Sleep(DefaultRequestCeiling)
			synctest.Wait()
			select {
			case got := <-done:
				if got.resp != nil || !errors.Is(got.err, ErrResponseHeaderTimeout) {
					t.Fatalf("default budget %d: response=%v error=%v", ms, got.resp, got.err)
				}
				if got.elapsed != DefaultRequestCeiling {
					t.Fatalf("default header wait = %v, want %v", got.elapsed, DefaultRequestCeiling)
				}
			default:
				cancel() // Also let a known-bad unbounded implementation exit cleanly.
				synctest.Wait()
				t.Fatal("default header budget did not expire")
			}
			if req.Context().Err() != nil {
				t.Fatal("header timeout cancelled the caller's context")
			}
		})
	}
}

func TestDoWithHeaderBudgetStopsAtHeadersAndReleasesOnClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://upstream.invalid", nil)
		var requestContext context.Context
		closeErr := errors.New("body close failed")
		resp, err := DoWithHeaderBudget(req, 60000, func(req *http.Request) (*http.Response, error) {
			requestContext = req.Context()
			time.Sleep(35 * time.Second) // Legitimate headers arrive after the old 30s cap.
			return &http.Response{Body: &budgetBody{
				Reader: strings.NewReader("data: still streaming\n\n"),
				close: func() error {
					// Close must cancel first, not wait for a blocked underlying Close.
					<-requestContext.Done()
					return closeErr
				},
			}}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Minute)
		if requestContext.Err() != nil {
			t.Fatalf("header timer cancelled a healthy body: %v", requestContext.Err())
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil || string(body) != "data: still streaming\n\n" {
			t.Fatalf("body=%q error=%v", body, err)
		}
		if err := resp.Body.Close(); !errors.Is(err, closeErr) {
			t.Fatalf("body Close error = %v, want %v", err, closeErr)
		}
		if requestContext.Err() != context.Canceled || req.Context().Err() != nil {
			t.Fatalf("close context ownership: child=%v parent=%v", requestContext.Err(), req.Context().Err())
		}
	})
}

func TestDoWithHeaderBudgetExplicitTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://upstream.invalid", nil)
		started := time.Now()
		resp, err := DoWithHeaderBudget(req, 25, func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		})
		if resp != nil || !errors.Is(err, ErrResponseHeaderTimeout) {
			t.Fatalf("response=%v error=%v, want header timeout", resp, err)
		}
		if elapsed := time.Since(started); elapsed != 25*time.Millisecond {
			t.Fatalf("timeout after %v, want 25ms", elapsed)
		}
	})
}

func TestDoWithHeaderBudgetPreservesCallerCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			want := context.Canceled
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
				want = context.DeadlineExceeded
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			defer cancel()
			req := httptest.NewRequest(http.MethodGet, "http://upstream.invalid", nil).WithContext(ctx)
			resp, err := DoWithHeaderBudget(req, 25, func(req *http.Request) (*http.Response, error) {
				<-req.Context().Done()
				return nil, req.Context().Err()
			})
			if resp != nil || !errors.Is(err, want) || errors.Is(err, ErrResponseHeaderTimeout) {
				t.Fatalf("response=%v error=%v, want caller error %v", resp, err, want)
			}
		})
	}
}

func TestDoWithHeaderBudgetRejectsLateHeadersAndClosesBody(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://upstream.invalid", nil)
		closed := false
		resp, err := DoWithHeaderBudget(req, 25, func(req *http.Request) (*http.Response, error) {
			// A RoundTripper may return a response while cancellation is in flight.
			time.Sleep(50 * time.Millisecond)
			return &http.Response{Body: &budgetBody{
				Reader: strings.NewReader("too late"),
				close:  func() error { closed = true; return nil },
			}}, nil
		})
		if resp != nil || !errors.Is(err, ErrResponseHeaderTimeout) || !closed {
			t.Fatalf("late response=%v error=%v closed=%v", resp, err, closed)
		}
	})
}

func TestDoWithHeaderBudgetTransportErrorReleasesContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://upstream.invalid", nil)
		want := errors.New("dial failed")
		var requestContext context.Context
		resp, err := DoWithHeaderBudget(req, 25, func(req *http.Request) (*http.Response, error) {
			requestContext = req.Context()
			return nil, want
		})
		if resp != nil || !errors.Is(err, want) || requestContext.Err() != context.Canceled {
			t.Fatalf("response=%v error=%v request context=%v", resp, err, requestContext.Err())
		}
	})
}

func TestDoWithHeaderBudgetSpansRedirects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		requests := 0
		client := &http.Client{Transport: budgetRoundTrip(func(req *http.Request) (*http.Response, error) {
			requests++
			timer := time.NewTimer(600 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-timer.C:
			}
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": {"/next"}},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		})}
		req, err := http.NewRequest(http.MethodGet, "http://upstream.invalid", nil)
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		resp, err := DoWithHeaderBudget(req, 1000, client.Do)
		if resp != nil || !errors.Is(err, ErrResponseHeaderTimeout) || requests != 2 {
			t.Fatalf("response=%v error=%v requests=%d", resp, err, requests)
		}
		if elapsed := time.Since(started); elapsed != time.Second {
			t.Fatalf("redirect reset header budget: elapsed=%v", elapsed)
		}
	})
}
