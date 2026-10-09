package httpclient

import (
	"net/http"
	"testing"
)

func TestExpandClientHeaderTemplate(t *testing.T) {
	client := http.Header{"X-Request-Id": {"request-42"}, "Authorization": {"private-value"}}
	for _, tt := range []struct {
		name, template, want string
		valid                bool
	}{
		{"literal", "static-value", "static-value", true},
		{"metadata", "prefix-{CLIENT_HEADER: X-Request-ID }-suffix", "prefix-request-42-suffix", true},
		{"multiple", "{client_header:x-request-id}/{client_header:x-request-id}", "request-42/request-42", true},
		{"missing metadata", "{client_header:traceparent}", "", true},
		{"credential", "{client_header:authorization}", "", false},
		{"cookie", "{client_header:cookie}", "", false},
		{"unterminated", "prefix-{client_header:x-request-id", "", false},
		{"empty name", "{client_header:}", "", false},
		{"reject partial result", "{client_header:x-request-id}/{client_header:authorization}", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := ExpandClientHeaderTemplate(tt.template, client)
			if got != tt.want || valid != tt.valid {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, valid, tt.want, tt.valid)
			}
			if _, accepted := ExpandClientHeaderTemplate(tt.template, nil); accepted != valid {
				t.Fatal("import validation and forwarding disagree")
			}
		})
	}
}
