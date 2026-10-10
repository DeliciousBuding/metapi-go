package proxyhandler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The fixture writes AWS EventStream binary frames directly, independent of
// the production reader. CRCs cover the wire bytes, not decoded JSON.
func bedrockTestFrame(headers [][2]string, payload []byte) []byte {
	var h bytes.Buffer
	for _, entry := range headers {
		h.WriteByte(byte(len(entry[0])))
		h.WriteString(entry[0])
		h.WriteByte(7)
		_ = binary.Write(&h, binary.BigEndian, uint16(len(entry[1])))
		h.WriteString(entry[1])
	}
	return bedrockTestRawFrame(h.Bytes(), payload)
}

func bedrockTestRawFrame(headers, payload []byte) []byte {
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, uint32(16+len(headers)+len(payload)))
	_ = binary.Write(&out, binary.BigEndian, uint32(len(headers)))
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(out.Bytes()))
	out.Write(headers)
	out.Write(payload)
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(out.Bytes()))
	return out.Bytes()
}

func bedrockTestChunk(data string) []byte {
	payload, _ := json.Marshal(map[string]string{"bytes": base64.StdEncoding.EncodeToString([]byte(data)), "p": "padding"})
	return bedrockTestFrame([][2]string{{":message-type", "event"}, {":event-type", "chunk"}, {":content-type", "application/json"}}, payload)
}

func bedrockTestStream(events ...string) []byte {
	var out []byte
	for _, event := range events {
		out = append(out, bedrockTestChunk(event)...)
	}
	return out
}

func TestDirectBedrockRequestURL(t *testing.T) {
	model := "arn:aws:bedrock:us-east-1:123456789012:inference-profile/anthropic.claude:0"
	for _, stream := range []bool{false, true} {
		target := directBedrockRequestURL("https://bedrock.example/gateway/model/?route=a", model, stream)
		req, err := http.NewRequest(http.MethodPost, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		action := "/invoke"
		if stream {
			action += "-with-response-stream"
		}
		if req.URL.Path != "/gateway/model/"+model+action || req.URL.RawQuery != "route=a" || !strings.Contains(req.URL.EscapedPath(), "inference-profile%2Fanthropic") {
			t.Fatalf("wrong URL: %s", target)
		}
	}
}

func TestDirectBedrockHTTPFixture(t *testing.T) {
	events := []string{
		`{"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","model":"anthropic.claude","content":[],"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":4}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"think\ncarefully"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"opaque_signature"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"id\":9007199254740993}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":20,"cache_creation_input_tokens":6}}`,
		`{"type":"message_stop"}`,
	}
	jsonResponse := `{"id":"msg_fixture","type":"message","role":"assistant","model":"anthropic.claude","content":[{"type":"thinking","thinking":"reason","signature":"opaque"},{"type":"tool_use","id":"tool_1","name":"lookup","input":{"id":9007199254740993}}],"stop_reason":"tool_use","usage":{"input_tokens":12,"output_tokens":20,"cache_read_input_tokens":4}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("Anthropic-Version") != "bedrock-2023-05-31" || r.Header.Get("Anthropic-Beta") != "" {
			t.Error("incorrect request headers")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != nil || body["stream"] != nil || string(body["anthropic_version"]) != `"bedrock-2023-05-31"` {
			t.Errorf("invalid Bedrock request: %v", body)
		}
		if r.URL.Path == "/model/anthropic.claude/invoke" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, jsonResponse)
			return
		}
		if r.URL.Path != "/model/anthropic.claude/invoke-with-response-stream" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream; charset=utf-8")
		for _, event := range events {
			frame := bedrockTestChunk(event)
			// Split prelude, headers and payload across writes.
			for offset := 0; offset < len(frame); offset += 7 {
				end := min(offset+7, len(frame))
				_, _ = w.Write(frame[offset:end])
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer server.Close()
	for _, stream := range []bool{false, true} {
		body, err := prepareDirectBedrockRequest([]byte(`{"model":"ignored","stream":true,"max_tokens":100,"messages":[{"role":"user","content":"hello"}]}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(http.MethodPost, directBedrockRequestURL(server.URL+"/model", "anthropic.claude", stream), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer fixture-token")
		buildDirectBedrockHeaders(req.Header)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := normalizeDirectBedrockResponse(req.Context(), resp, stream); err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !stream {
			if string(got) != jsonResponse {
				t.Fatalf("JSON changed: %s", got)
			}
			continue
		}
		var want strings.Builder
		for _, event := range events {
			var value struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(event), &value)
			want.WriteString("event: " + value.Type + "\ndata: " + event + "\n\n")
		}
		if string(got) != want.String() {
			t.Fatalf("stream fields changed:\n%s", got)
		}
		if resp.Header.Get("Content-Type") != "text/event-stream" || resp.ContentLength != -1 {
			t.Fatal("response metadata not normalized")
		}
		analyzer := newIncrementalSseAnalyzer()
		analyzer.Push(got)
		usage := analyzer.Result().Usage
		if !usage.Found || usage.CompletionTokens != 20 || usage.CacheReadTokens != 4 || usage.CacheCreationTokens != 6 {
			t.Fatalf("shared parser lost Bedrock usage: %+v", usage)
		}
	}
}

func TestDirectBedrockInvalidFrames(t *testing.T) {
	start := bedrockTestChunk(`{"type":"message_start","message":{}}`)
	badPrelude := append([]byte(nil), start...)
	badPrelude[8] ^= 1
	badMessage := append([]byte(nil), start...)
	badMessage[len(badMessage)-1] ^= 1
	oversize := append([]byte(nil), start[:12]...)
	binary.BigEndian.PutUint32(oversize[:4], directBedrockMaxPayload+directBedrockMaxHeaders+17)
	binary.BigEndian.PutUint32(oversize[8:], crc32.ChecksumIEEE(oversize[:8]))
	header := [][2]string{{":message-type", "event"}, {":event-type", "chunk"}}
	cases := map[string][]byte{
		"empty": nil, "prelude CRC": badPrelude, "message CRC": badMessage,
		"truncated prelude": start[:8], "truncated payload": start[:len(start)-3], "truncated no payload": start[:12], "oversize": oversize,
		"missing routing headers": bedrockTestFrame(nil, []byte(`{}`)),
		"duplicate header":        bedrockTestFrame(append(header, [2]string{":message-type", "event"}), []byte(`{}`)),
		"invalid base64":          bedrockTestFrame(header, []byte(`{"bytes":"!!"}`)),
		"invalid JSON":            bedrockTestChunk(`{`),
		"injection type":          bedrockTestChunk(`{"type":"message_stop\ndata: evil"}`),
		"unknown AWS event":       bedrockTestFrame([][2]string{{":message-type", "event"}, {":event-type", "other"}}, []byte(`{}`)),
		"before start":            bedrockTestChunk(`{"type":"message_stop"}`),
		"no stop":                 start,
		"duplicate start":         append(append([]byte(nil), start...), start...),
		"after stop":              bedrockTestStream(`{"type":"message_start"}`, `{"type":"message_stop"}`, `{"type":"ping"}`),
		"exception":               bedrockTestFrame([][2]string{{":message-type", "exception"}, {":exception-type", "throttlingException"}}, []byte(`{"message":"quota exceeded"}`)),
		"error":                   bedrockTestFrame([][2]string{{":message-type", "error"}, {":error-code", "InternalFailure"}, {":error-message", "try later"}}, nil),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			reader := newDirectBedrockStream(context.Background(), io.NopCloser(bytes.NewReader(data)))
			defer reader.Close()
			_, err := io.ReadAll(reader)
			if err == nil {
				t.Fatal("accepted invalid stream")
			}
			if (name == "prelude CRC" || name == "message CRC") && !strings.Contains(err.Error(), name) {
				t.Fatalf("wrong CRC layer: %v", err)
			}
			if name == "exception" && (!strings.Contains(err.Error(), "throttlingException") || !strings.Contains(err.Error(), "quota exceeded")) {
				t.Fatal(err)
			}
			if name == "error" && (!strings.Contains(err.Error(), "InternalFailure") || !strings.Contains(err.Error(), "try later")) {
				t.Fatal(err)
			}
		})
	}
}

func TestDirectBedrockCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	stream := newDirectBedrockStream(ctx, reader)
	defer stream.Close()
	result := make(chan error, 1)
	go func() { _, err := io.ReadAll(stream); result <- err }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not unblock body read")
	}
}

func TestDirectBedrockFailureRetainsObservedUsage(t *testing.T) {
	data := bedrockTestChunk(`{"type":"message_start","message":{"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":4}}}`)
	data = append(data, bedrockTestFrame([][2]string{{":message-type", "exception"}, {":exception-type", "modelStreamErrorException"}}, []byte(`{"message":"upstream failed"}`))...)
	reader := newDirectBedrockStream(context.Background(), io.NopCloser(bytes.NewReader(data)))
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err == nil || bytes.Contains(got, []byte("message_stop")) {
		t.Fatalf("failure turned into completion: %s, %v", got, err)
	}
	analyzer := newIncrementalSseAnalyzer()
	analyzer.Push(got)
	usage := analyzer.Result().Usage
	if !usage.Found || usage.CompletionTokens != 1 || usage.CacheReadTokens != 4 {
		t.Fatalf("lost partial usage: %+v", usage)
	}
	_, nextErr := reader.Read(make([]byte, 1))
	if nextErr == nil || nextErr.Error() != err.Error() {
		t.Fatalf("error not sticky: %v / %v", err, nextErr)
	}
}

func TestDirectBedrockCompressedStream(t *testing.T) {
	data := bedrockTestStream(`{"type":"message_start"}`, `{"type":"message_stop"}`)
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, _ = zw.Write(data)
	_ = zw.Close()
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/vnd.amazon.eventstream"}, "Content-Encoding": {"gzip"}}, Body: io.NopCloser(bytes.NewReader(compressed.Bytes()))}
	if err := normalizeDirectBedrockResponse(context.Background(), resp, true); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || !bytes.Contains(body, []byte("event: message_stop\n")) {
		t.Fatalf("got %s, %v", body, err)
	}
	if resp.Header.Get("Content-Encoding") != "" {
		t.Fatal("stale content encoding")
	}
}

func TestDirectBedrockNormalizationDoesNotReadBody(t *testing.T) {
	for _, encoding := range []string{"", "gzip"} {
		reader, writer := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/vnd.amazon.eventstream"}, "Content-Encoding": {encoding}}, Body: reader}
		if err := normalizeDirectBedrockResponse(ctx, resp, true); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := io.ReadAll(resp.Body); done <- err }()
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s: %v", encoding, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("compressed body did not cancel")
		}
		_ = resp.Body.Close()
		_ = writer.Close()
	}
}

func TestDirectBedrockUnexpectedResponse(t *testing.T) {
	for _, ct := range []string{"", "application/json", "text/event-stream"} {
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {ct}}, Body: io.NopCloser(strings.NewReader(`{}`))}
		if err := normalizeDirectBedrockResponse(context.Background(), resp, true); err == nil {
			t.Fatalf("accepted %q", ct)
		}
	}
	resp := &http.Response{StatusCode: 429, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"message":"throttled"}`))}
	if err := normalizeDirectBedrockResponse(context.Background(), resp, true); err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatal("HTTP error mutated")
	}
}

func TestDirectBedrockHeaderTypes(t *testing.T) {
	var raw bytes.Buffer
	for typ := byte(0); typ <= 9; typ++ {
		raw.WriteByte(1)
		raw.WriteByte('a' + typ)
		raw.WriteByte(typ)
		size := []int{0, 0, 1, 2, 4, 8, 3, 3, 8, 16}[typ]
		if typ == 6 || typ == 7 {
			raw.Write([]byte{0, byte(size)})
		}
		raw.Write(bytes.Repeat([]byte{'x'}, size))
	}
	headers, err := readDirectBedrockHeaders(raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(headers, map[string]string{"h": "xxx"}) {
		t.Fatal(headers)
	}
	for _, bad := range [][]byte{{0}, {1, 'a', 10}, {1, 'a', 7, 0}, {1, 'a', 7, 0, 2, 'x'}, {1, 'a', 7, 0, 1, 255}, {1, ':', 0}} {
		if _, err := readDirectBedrockHeaders(bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}
