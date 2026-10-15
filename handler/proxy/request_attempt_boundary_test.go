package proxyhandler

import (
	"net/http"
	"strings"
	"testing"
)

func TestRequestAttemptStatusDoesNotRepresentCommittedHTTPStatus(t *testing.T) {
	upstream := abruptSSEUpstream(`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n")
	defer upstream.Close()
	_, logs, response := runStreamDispatch(t, upstream.URL, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "partial") {
		t.Fatalf("downstream prefix was not committed: status=%d body=%s", response.Code, response.Body.String())
	}
	if len(*logs) != 1 || (*logs)[0].Status == "success" || (*logs)[0].HTTPStatus == response.Code {
		t.Fatalf("stream failure must remain an attempt observation, separate from committed HTTP status: %+v", *logs)
	}
}
