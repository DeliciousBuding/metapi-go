package proxyhandler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type imageSessionCountingBody struct {
	io.Reader
	closes atomic.Int32
}

func (b *imageSessionCountingBody) Close() error {
	b.closes.Add(1)
	return nil
}

// Replace only transport so the production begin/query paths still acquire the
// response and own its lifetime. No DB or protocol behavior is mocked here.
func imageSessionTransport(t *testing.T, roundTrip domesticTestTransport) {
	t.Helper()
	previous := defaultUpstreamClient
	defaultUpstreamClient = &http.Client{Transport: roundTrip}
	t.Cleanup(func() { defaultUpstreamClient = previous })
}

func TestProviderImageSessionClosesSubmissionBeforeFirstRead(t *testing.T) {
	body := &imageSessionCountingBody{Reader: strings.NewReader(`{}`)}
	imageSessionTransport(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, Request: r}, nil
	})
	op := directImageTaskOperation{wire: &directProviderWire{}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://image.example/images/generations", nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := op.begin(req)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	if err = session.Close(); err != nil || body.closes.Load() != 1 {
		t.Fatalf("unread submission body was not closed exactly once: closes=%d err=%v", body.closes.Load(), err)
	}
}

func TestProviderImageSessionClosesQueryBeforeFirstRead(t *testing.T) {
	for _, closeDuringHeaders := range []bool{false, true} {
		t.Run(map[bool]string{false: "after-headers", true: "close-before-publish"}[closeDuringHeaders], func(t *testing.T) {
			op := imageOperationFixture(t, "https://image.example", imageOperationModule)
			op.cfg = nil
			ctx, cancel := context.WithCancel(t.Context())
			session := &directImageTaskSession{op: op, ctx: ctx, cancel: cancel, original: &http.Request{Header: make(http.Header)}, taskID: "one"}
			defer session.Close()
			body := &imageSessionCountingBody{Reader: strings.NewReader(`{}`)}
			imageSessionTransport(t, func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.URL.Path != "/tasks/one" {
					t.Error("query changed the saved operation")
				}
				if closeDuringHeaders {
					_ = session.Close()
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, Request: r}, nil
			})
			err := session.query()
			if closeDuringHeaders && !errors.Is(err, context.Canceled) || !closeDuringHeaders && err != nil {
				t.Errorf("query cancellation result: %v", err)
			}
			_ = session.Close()
			_ = session.Close()
			if body.closes.Load() != 1 {
				t.Fatalf("unread query body was not closed exactly once: closes=%d", body.closes.Load())
			}
		})
	}
}

func TestProviderImageSessionStopsBufferedFrameOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	session := &directImageTaskSession{ctx: ctx, cancel: cancel, usage: ParsedUsage{Found: true, PromptTokens: 7, TotalTokens: 7}}
	body := &directImageTaskBody{session: session, pending: []byte("event: image_generation.partial_image\ndata: fixture-frame\n\n")}
	defer body.Close()
	buffer := make([]byte, 1)
	if n, err := body.Read(buffer); n != 1 || err != nil {
		t.Fatalf("first frame byte: n=%d err=%v", n, err)
	}
	cancel()
	n, err := body.Read(buffer)
	if n != 0 || !errors.Is(err, context.Canceled) || len(body.pending) != 0 || directMediaFailureUsage(err).TotalTokens != 7 {
		t.Fatalf("canceled stream returned cached bytes or lost usage: n=%d pending=%d usage=%+v err=%v", n, len(body.pending), directMediaFailureUsage(err), err)
	}
}

type imageSessionWaitReader struct {
	ctx     context.Context
	entered chan struct{}
}

func (r imageSessionWaitReader) Read([]byte) (int, error) {
	close(r.entered)
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func TestProviderImageSessionConcurrentReadAndCloseOwnOneBody(t *testing.T) {
	entered := make(chan struct{})
	body := &imageSessionCountingBody{}
	var posts atomic.Int32
	imageSessionTransport(t, func(r *http.Request) (*http.Response, error) {
		posts.Add(1)
		body.Reader = imageSessionWaitReader{ctx: r.Context(), entered: entered}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, Request: r}, nil
	})
	op := directImageTaskOperation{wire: &directProviderWire{}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://image.example/images/generations", nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := op.begin(req)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	session.usage = ParsedUsage{Found: true, PromptTokens: 7, TotalTokens: 7}
	stream := &directImageTaskBody{session: session}
	result := make(chan error, 1)
	go func() { _, err := stream.Read(make([]byte, 1)); result <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("raw response Read did not start")
	}
	_ = stream.Close()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || directMediaFailureUsage(err).TotalTokens != 7 {
			t.Fatalf("concurrent close lost cancellation or original usage: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("raw response Read outlived Close")
	}
	_ = stream.Close()
	if body.closes.Load() != 1 || posts.Load() != 1 {
		t.Fatalf("concurrent cleanup duplicated Close or POST: closes=%d posts=%d", body.closes.Load(), posts.Load())
	}
}
