package proxyhandler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestRequestBodyReadDeadlinePreservesAdmissionReason(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		incomplete              bool
		status                  int
		reason                  string
	}{
		{"json-timeout", "application/json", `{"model":"test-model","messages":[`, true, http.StatusRequestTimeout, "request_body_timeout"},
		{"multipart-timeout", "multipart/form-data; boundary=boundary", "--boundary\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\ntest-model\r\n--boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"partial.txt\"\r\n\r\npartial", true, http.StatusRequestTimeout, "request_body_timeout"},
		{"invalid-json", "application/json", `{"model":`, false, http.StatusBadRequest, "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := make(chan store.GatewayRequest, 1)
			handler := requestLifecycleMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{
					Token: "synthetic-key", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy,
				}))
				ctx, failure := PrepareCtx(r, SurfConfig{DownstreamPath: r.URL.Path, RequireModel: true})
				if ctx != nil || failure == nil {
					t.Error("incomplete or malformed body was admitted")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				writeJSONError(w, failure.Status, failure.Error, failure.ErrorType)
			}), func(_ context.Context, record store.GatewayRequest) error {
				records <- record
				return nil
			})
			server := httptest.NewUnstartedServer(handler)
			server.Config.ReadTimeout = 150 * time.Millisecond
			server.Start()
			defer server.Close()
			conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			length := len(tc.body)
			if tc.incomplete {
				length += 100
			}
			_, err = fmt.Fprintf(conn, "POST /v1/chat/completions HTTP/1.1\r\nHost: localhost\r\nContent-Type: %s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", tc.contentType, length, tc.body)
			if err != nil {
				t.Fatal(err)
			}
			response, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != tc.status {
				t.Fatalf("status=%d body=%q error=%v", response.StatusCode, body, err)
			}
			if tc.incomplete && !strings.Contains(string(body), "request body read timed out") {
				t.Fatalf("timeout was hidden: %s", body)
			}
			select {
			case record := <-records:
				if record.ReasonCode == nil || *record.ReasonCode != tc.reason || record.TerminalStatus != "rejected_policy" ||
					record.HTTPStatus == nil || *record.HTTPStatus != tc.status || record.AttemptCount != 0 || record.ModelRequested != nil {
					t.Fatalf("admission record=%+v", record)
				}
			case <-time.After(time.Second):
				t.Fatal("admission record missing")
			}
		})
	}
}
