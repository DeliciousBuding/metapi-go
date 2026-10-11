package messages_test

import (
	"bytes"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

func TestToChatRequestImagesPreserveOrderAndReferences(t *testing.T) {
	body := []byte(`{"model":"vision-model","max_tokens":256,"messages":[{"role":"user","content":[{"type":"text","text":"before"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AQID"}},{"type":"text","text":"between"},{"type":"image","source":{"type":"url","url":"https://images.example/a.webp?key=public%2Fimage"}},{"type":"text","text":"after"}]}]}`)
	chat, err := messages.ToChatRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	requireJSON(t, chat, `{"model":"vision-model","max_tokens":256,"messages":[{"role":"user","content":[{"type":"text","text":"before"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AQID"}},{"type":"text","text":"between"},{"type":"image_url","image_url":{"url":"https://images.example/a.webp?key=public%2Fimage"}},{"type":"text","text":"after"}]}]}`)
	again, err := messages.FromChatRequest(chat)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodeJSON(t, body), decodeJSON(t, again)) {
		t.Fatalf("native image roundtrip changed content: %s", again)
	}
	// Exercise the actual cross-protocol composition without handler coupling.
	converted, err := responses.FromChatRequest(chat)
	if err != nil {
		t.Fatal(err)
	}
	input := jsonArray(t, jsonObject(t, decodeJSON(t, converted))["input"])
	content := jsonArray(t, jsonObject(t, input[0])["content"])
	if len(content) != 5 || jsonObject(t, content[1])["type"] != "input_image" || jsonObject(t, content[1])["image_url"] != "data:image/png;base64,AQID" || jsonObject(t, content[3])["image_url"] != "https://images.example/a.webp?key=public%2Fimage" {
		t.Fatalf("Messages to Responses dropped images: %s", converted)
	}
}

func TestToChatRequestImagesPreserveReasoningReplay(t *testing.T) {
	const hidden = "fixture hidden reasoning"
	var saved map[string]string = make(map[string]string)
	var loadIDs []string
	options := messages.Options{
		SaveReasoning: func(ids []string, value string) error { saved[strings.Join(ids, "\x00")] = value; return nil },
		LoadReasoning: func(ids []string) (string, bool, error) {
			loadIDs = append([]string(nil), ids...)
			value, ok := saved[strings.Join(ids, "\x00")]
			return value, ok, nil
		},
	}
	reply, err := messages.FromChatResponse(hiddenToolReply(t, hidden), options)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reply, []byte(hidden)) {
		t.Fatal("hidden reasoning leaked into native reply")
	}
	req := reasoningContinuation(t, reply)
	transcript := req["messages"].([]any)
	user := transcript[len(transcript)-1].(map[string]any)
	user["content"] = append(user["content"].([]any), map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://images.example/result.png"}}, map[string]any{"type": "text", "text": "compare with both tool results"})
	chat, err := messages.ToChatRequest(marshalJSON(t, req), options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loadIDs, []string{"call-1", "call-2"}) {
		t.Fatalf("replay group changed: %v", loadIDs)
	}
	items := jsonArray(t, jsonObject(t, decodeJSON(t, chat))["messages"])
	var assistant map[string]any
	for _, item := range items {
		m := jsonObject(t, item)
		if m["role"] == "assistant" {
			assistant = m
		}
	}
	if assistant["reasoning_content"] != hidden || !bytes.Contains(chat, []byte("https://images.example/result.png")) {
		t.Fatalf("reasoning/image lost: %s", chat)
	}
	if bytes.Contains(chat, []byte(`"signature"`)) {
		t.Fatal("invented signature")
	}
	if _, err := responses.FromChatRequest(chat); err != nil {
		t.Fatalf("replayed image cannot reach Responses: %v", err)
	}
	if out, err := messages.ToChatRequest(marshalJSON(t, req)); !errors.Is(err, messages.ErrReasoningReplay) || len(out) > 0 {
		t.Fatal("missing replay was ignored for image continuation")
	}
}

func TestToChatRequestInvalidImages(t *testing.T) {
	for _, source := range []string{
		`{}`, `{"type":"file","file_id":"file-1"}`, `{"type":"base64","media_type":"image/png","data":"AB=="}`, `{"type":"base64","media_type":"image/png","data":"!bad"}`, `{"type":"base64","media_type":"image/png","data":""}`, `{"type":"base64","media_type":"image/svg+xml","data":"AQID"}`, `{"type":"url","url":"data:image/png;base64,AQID"}`, `{"type":"url","url":"/relative.png"}`, `{"type":"url","url":"file:///a.png"}`, `{"type":"url","url":"https://images.example/a b.png"}`, `{"type":"url","url":"https://images.example/a.png","headers":{"Authorization":"fixture"}}`,
	} {
		body := []byte(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":[{"type":"image","source":` + source + `}]}]}`)
		if out, err := messages.ToChatRequest(body); err == nil || len(out) > 0 {
			t.Errorf("accepted invalid image %s", source)
		}
	}
	credentialURL := &url.URL{Scheme: "https", Host: "images.example", Path: "/i.png", User: url.UserPassword("fixture-user", "fixture-password")}
	for _, extra := range []map[string]any{
		{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}},
		{"source": map[string]any{"type": "url", "url": credentialURL.String()}},
		{"detail": "high"}, {"signature": "opaque"},
	} {
		image := map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://images.example/i.png"}}
		for k, v := range extra {
			image[k] = v
		}
		body := marshalJSON(t, map[string]any{"model": "m", "max_tokens": 64, "messages": []any{map[string]any{"role": "user", "content": []any{image}}}})
		if out, err := messages.ToChatRequest(body); err == nil || len(out) > 0 {
			t.Errorf("dropped unsupported image semantics: %s", out)
		}
	}
}

func TestToChatRequestImagesCannotReorderToolsOrRoles(t *testing.T) {
	const image = `{"type":"image","source":{"type":"url","url":"https://images.example/i.png"}}`
	const call = `{"type":"tool_use","id":"call-1","name":"inspect","input":{"id":9007199254740993}}`
	for _, transcript := range []string{
		`[{"role":"user","content":"hi"},{"role":"assistant","content":[` + image + `]},{"role":"user","content":"continue"}]`,
		`[{"role":"assistant","content":[` + call + `]},{"role":"user","content":[` + image + `,{"type":"tool_result","tool_use_id":"call-1","content":"ok"}]}]`,
		`[{"role":"assistant","content":[` + call + `]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":[` + image + `]}]}]`,
		`[{"role":"user","content":[` + image + `]},{"role":"assistant","content":[{"type":"thinking","thinking":"opaque history","signature":"signed"}]},{"role":"user","content":"continue"}]`,
	} {
		if out, err := messages.ToChatRequest([]byte(`{"model":"m","max_tokens":64,"messages":` + transcript + `}`)); err == nil || len(out) > 0 {
			t.Fatalf("accepted unrepresentable image history: %s", out)
		}
	}
}
