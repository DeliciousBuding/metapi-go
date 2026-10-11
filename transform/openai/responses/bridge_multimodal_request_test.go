package responses

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
)

func TestChatBridgeMultimodalRequestRoundTrip(t *testing.T) {
	input := []byte(`{"model":"vision-model","input":[{"role":"user","content":[{"type":"input_text","text":"Before"},{"type":"input_image","image_url":"https://images.example/a.png?version=2%2F3","detail":"original"},{"type":"input_text","text":"Between"},{"type":"input_image","image_url":"data:image/png;base64,AQID","detail":"low"},{"type":"input_text","text":"After"}]},{"type":"reasoning","summary":[{"type":"summary_text","text":"inspect then call"}]},{"type":"function_call","call_id":"call-1","name":"inspect","arguments":"{\"id\":9007199254740993}"},{"type":"function_call_output","call_id":"call-1","output":"observed"},{"role":"user","content":[{"type":"input_image","image_url":"https://images.example/b.webp"}]}],"text":{"format":{"type":"json_schema","name":"Image_Result-v1","description":"Extract image labels","schema":{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993},"label":{"$ref":"#/$defs/label"}},"$defs":{"label":{"type":"string"}},"required":["id","label"],"additionalProperties":false},"strict":true},"verbosity":"low"},"tools":[{"type":"function","name":"inspect","parameters":{"type":"object","properties":{"id":{"const":9007199254740993}}},"strict":false}],"reasoning":{"effort":"high"}}`)
	chat, err := ToChatRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	obj := bridgeTestObject(t, chat)
	msgs := obj["messages"].([]any)
	parts := bridgeMap(msgs[0])["content"].([]any)
	if len(parts) != 5 || bridgeMap(parts[0])["text"] != "Before" || bridgeMap(parts[2])["text"] != "Between" || bridgeMap(parts[4])["text"] != "After" {
		t.Fatalf("content order changed: %s", chat)
	}
	firstImage := bridgeMap(bridgeMap(parts[1])["image_url"])
	if firstImage["url"] != "https://images.example/a.png?version=2%2F3" || firstImage["detail"] != "original" {
		t.Fatalf("image reference/detail changed: %s", chat)
	}
	if bridgeMap(msgs[1])["reasoning_content"] != "inspect then call" || bridgeMap(msgs[2])["tool_call_id"] != "call-1" {
		t.Fatalf("tool reasoning shifted: %s", chat)
	}
	if !bytes.Contains(chat, []byte(`"minimum":9007199254740993`)) || !bytes.Contains(chat, []byte(`"const":9007199254740993`)) {
		t.Fatalf("schema integer lost: %s", chat)
	}
	format := bridgeMap(obj["response_format"])
	if format["type"] != "json_schema" || bridgeMap(format["json_schema"])["strict"] != true || obj["verbosity"] != "low" {
		t.Fatalf("structured output lost: %s", chat)
	}
	again, err := FromChatRequest(chat)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ToChatRequest(again)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bridgeTestJSON(t, obj), bridgeTestJSON(t, bridgeTestObject(t, replayed))) {
		t.Fatalf("roundtrip changed request:\n%s\n%s", chat, replayed)
	}
}

func TestChatBridgeImageDetailsAndFormats(t *testing.T) {
	for _, detail := range []string{"auto", "low", "high", "original"} {
		for _, media := range []string{"image/png", "image/jpeg", "image/gif", "image/webp"} {
			input := bridgeObject{"model": "m", "input": []any{bridgeObject{"role": "user", "content": []any{bridgeObject{"type": "input_image", "image_url": "data:" + media + ";base64,AQID", "detail": detail}}}}}
			chat, err := ToChatRequest(bridgeTestJSON(t, input))
			if err != nil {
				t.Fatal(err)
			}
			out, err := FromChatRequest(chat)
			if err != nil {
				t.Fatal(err)
			}
			// FromChatRequest explicitly annotates message type and stateless mode.
			got := bridgeTestObject(t, out)
			delete(bridgeMap(got["input"].([]any)[0]), "type")
			delete(got, "store")
			if !bytes.Equal(bridgeTestJSON(t, input), bridgeTestJSON(t, got)) {
				t.Fatalf("image changed: %s", out)
			}
		}
	}
}

func TestChatBridgeRejectsInvalidImages(t *testing.T) {
	bad := []bridgeObject{
		{"image_url": "https://images.example/i.png", "file_id": "file-1"},
		{"file_id": "file-1"}, {"image_url": "/relative.png"}, {"image_url": "file:///image.png"},
		{"image_url": "https://images.example/i.png", "detail": "highest"}, {"image_url": "https://images.example/i.png", "detail": 1},
		{"image_url": "https://images.example/i.png", "prompt_cache_breakpoint": bridgeObject{"mode": "explicit"}},
		{"image_url": "data:image/png;base64,!bad"}, {"image_url": "data:image/png;base64,AB=="},
		{"image_url": "data:image/svg+xml;base64,AQID"}, {"image_url": "data:image/png,AQID"},
		{"image_url": "data:image/png;base64,\r\n"}, {"image_url": "https://images.example/a b.png"},
		{"image_url": bridgeObject{"url": "https://images.example/i.png"}},
	}
	for i, part := range bad {
		part["type"] = "input_image"
		body := bridgeTestJSON(t, bridgeObject{"model": "m", "input": []any{bridgeObject{"role": "user", "content": []any{part}}}})
		if out, err := ToChatRequest(body); err == nil || len(out) > 0 {
			t.Errorf("case %d accepted invalid image: %s", i, out)
		}
	}
	for _, role := range []string{"assistant", "system", "developer"} {
		body := bridgeTestJSON(t, bridgeObject{"model": "m", "input": []any{bridgeObject{"role": role, "content": []any{bridgeObject{"type": "input_image", "image_url": "https://images.example/i.png"}}}}})
		if _, err := ToChatRequest(body); err == nil {
			t.Errorf("accepted %s image", role)
		}
	}
	for _, role := range []string{"assistant", "tool", "system", "developer"} {
		msg := bridgeObject{"role": role, "content": []any{bridgeObject{"type": "image_url", "image_url": bridgeObject{"url": "https://images.example/i.png"}}}}
		if role == "tool" {
			msg["tool_call_id"] = "c"
		}
		body := bridgeTestJSON(t, bridgeObject{"model": "m", "messages": []any{msg}})
		if _, err := FromChatRequest(body); err == nil {
			t.Errorf("accepted Chat %s image", role)
		}
	}
}

func TestChatBridgeTextFormats(t *testing.T) {
	for _, raw := range []string{`{}`, `{"format":{"type":"text"}}`, `{"format":{"type":"json_object"}}`, `{"format":{"type":"json_schema","name":"result","schema":{},"strict":false}}`, `{"format":{"type":"json_schema","name":"result","schema":{},"strict":null},"verbosity":"high"}`} {
		chat, err := ToChatRequest([]byte(`{"model":"m","input":"hi","text":` + raw + `}`))
		if err != nil {
			t.Fatal(err)
		}
		again, err := FromChatRequest(chat)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "{}" && !bytes.Equal(bridgeTestJSON(t, bridgeTestObject(t, []byte(raw))), bridgeTestJSON(t, bridgeTestObject(t, again)["text"])) {
			t.Fatalf("format changed: %s", again)
		}
	}
	for _, raw := range []string{`"bad"`, `{"format":[]}`, `{"format":{"type":"json_schema"}}`, `{"format":{"type":"json_schema","name":"bad name","schema":{}}}`, `{"format":{"type":"json_schema","name":"` + strings.Repeat("a", 65) + `","schema":{}}}`, `{"format":{"type":"json_schema","name":"valid","schema":[]}}`, `{"format":{"type":"json_schema","name":"valid","schema":{},"strict":1}}`, `{"format":{"type":"json_schema","name":"valid","schema":{},"description":1}}`, `{"format":{"type":"json_object","schema":{}}}`, `{"format":{"type":"grammar"}}`, `{"format":{"type":"text"},"verbosity":"extreme"}`, `{"format":{"type":"text"},"future":true}`} {
		if out, err := ToChatRequest([]byte(`{"model":"m","input":"hi","text":` + raw + `}`)); err == nil || len(out) > 0 {
			t.Errorf("accepted invalid format %s", raw)
		}
	}
	if _, err := FromChatRequest([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"r","schema":{},"strict":true,"future":1}}}`)); err == nil {
		t.Fatal("dropped unknown schema option")
	}
}

func TestChatBridgeImageDoesNotDropSignedReasoning(t *testing.T) {
	request := bridgeObject{"model": "m", "input": []any{bridgeObject{"role": "user", "content": []any{bridgeObject{"type": "input_image", "image_url": "https://images.example/i.png"}}}, bridgeObject{"type": "reasoning", "summary": []any{bridgeObject{"type": "summary_text", "text": "reason"}}, "encrypted_content": "opaque"}}}
	if out, err := ToChatRequest(bridgeTestJSON(t, request)); err == nil || len(out) > 0 {
		t.Fatal("image support weakened encrypted reasoning refusal")
	}
}

func TestChatBridgeImageURLCredentialsAreRejected(t *testing.T) {
	// Construct userinfo separately so the public fixture is not credential-like.
	location := &url.URL{Scheme: "https", Host: "images.example", Path: "/a.png", User: url.UserPassword("fixture-user", "fixture-password")}
	if err := bridgeImageURL(location.String()); err == nil {
		t.Fatal("embedded image credentials accepted")
	}
}
