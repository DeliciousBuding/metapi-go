package ollama

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPNativeChatExposesFirstChunkBeforeTerminal(t *testing.T) {
	finish := make(chan struct{})
	seenRequest := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seenRequest <- body
		if r.URL.Path != "/custom/api/chat" || r.Method != http.MethodPost {
			t.Errorf("unexpected native path/method: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"model":"qwen3:latest","done":false,"message":{"content":"first"}}`+"\n")
		w.(http.Flusher).Flush()
		select {
		case <-finish:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, `{"model":"qwen3:latest","done":false,"message":{"thinking":"reason"}}`+"\n")
		_, _ = io.WriteString(w, `{"model":"qwen3:latest","done":true,"done_reason":"stop","prompt_eval_count":9,"eval_count":4,"total_duration":9007199254740993}`+"\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	body, err := FromChatRequest([]byte(`{"model":"qwen3","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/custom/api/chat", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := NewNDJSONReader(ctx, resp.Body, "qwen3", StreamOptions{IdleTimeout: time.Second})
	defer r.Close()
	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil || !bytes.Contains(buf[:n], []byte(`"content":"first"`)) || bytes.Contains(buf[:n], []byte("[DONE]")) {
		t.Fatalf("no incremental HTTP output: %s %v", buf[:n], err)
	}
	if got := <-seenRequest; !bytes.Contains(got, []byte(`"stream":true`)) {
		t.Fatalf("wrong native request: %s", got)
	}
	close(finish)
	last, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"reasoning_content":"reason"`, `"total_tokens":13`, `"total_duration":9007199254740993`, "data: [DONE]\n\n"} {
		if !bytes.Contains(last, []byte(want)) {
			t.Fatalf("missing %s in %s", want, last)
		}
	}
}

func TestHTTPNativeErrorsAndTruncation(t *testing.T) {
	for name, body := range map[string]string{
		"200 native error": `{"error":"model not found"}` + "\n",
		"truncated":        `{"model":"qwen3","done":false,"message":{"content":"partial"}}` + "\n",
		"invalid line":     "{broken json}\n",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			resp, err := server.Client().Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			r := NewNDJSONReader(context.Background(), resp.Body, "qwen3", StreamOptions{})
			got, err := io.ReadAll(r)
			if err == nil || bytes.Contains(got, []byte("[DONE]")) {
				t.Fatalf("failed HTTP stream succeeded: %s %v", got, err)
			}
		})
	}
}

func TestHTTPReaderClosesUpstreamOnIdleOrCancel(t *testing.T) {
	for _, mode := range []string{"idle", "cancel", "line limit", "tiny line limit"} {
		t.Run(mode, func(t *testing.T) {
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-ndjson")
				w.WriteHeader(http.StatusOK)
				if strings.Contains(mode, "line limit") {
					_, _ = io.WriteString(w, strings.Repeat("x", 100))
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(closed)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			opts := StreamOptions{IdleTimeout: time.Second}
			if mode == "idle" {
				opts.IdleTimeout = 20 * time.Millisecond
			}
			if mode == "line limit" {
				opts.MaxLineBytes = 32
			}
			if mode == "tiny line limit" {
				opts.MaxLineBytes = 8
			}
			r := NewNDJSONReader(ctx, resp.Body, "qwen3", opts)
			result := make(chan error, 1)
			go func() { _, err := io.ReadAll(r); result <- err }()
			if mode == "cancel" {
				cancel()
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("stalled HTTP stream succeeded")
				}
				if mode == "idle" && !errors.Is(err, ErrIdleTimeout) {
					t.Fatalf("wrong idle error: %v", err)
				}
				if mode == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatalf("wrong cancellation error: %v", err)
				}
				if strings.Contains(mode, "line limit") && !strings.Contains(err.Error(), "line exceeds byte limit") {
					t.Fatalf("wrong limit error: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("HTTP read was not interrupted")
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("upstream connection stayed open")
			}
		})
	}
}
