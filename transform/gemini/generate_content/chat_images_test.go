package generate_content

import (
	"bytes"
	"encoding/json"
	"net/url"
	"testing"
)

func TestChatResponseImagesAndExactUsage(t *testing.T) {
	raw := []byte(`{"id":"fixture","model":"image-model","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","text":"first"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGk="}},{"type":"text","text":"last"},{"type":"image_url","image_url":{"url":"https://images.example/output.webp?size=1"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":9007199254740993,"completion_tokens":2,"total_tokens":9007199254740995,"completion_tokens_details":{"reasoning_tokens":1}}}`)
	out, err := FromChatResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"promptTokenCount":9007199254740993`, `"totalTokenCount":9007199254740995`, `"candidatesTokenCount":1`, `"mimeType":"image/png"`, `"data":"aGk="`, `"mimeType":"image/webp"`} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
	parsed, err := bridgeObject(out)
	if err != nil {
		t.Fatal(err)
	}
	parts := parsed["candidates"].([]any)[0].(map[string]any)["content"].(map[string]any)["parts"].([]any)
	if len(parts) != 4 || parts[0].(map[string]any)["text"] != "first" || parts[2].(map[string]any)["text"] != "last" {
		t.Fatalf("part order changed: %s", out)
	}
	stream := NewChatStream("image-model")
	frame := []byte("data: " + `{"id":"fixture","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,aGk="}}]}}]}` + "\n\n")
	out, err = stream.TransformEvent(frame)
	if err != nil || !bytes.Contains(out, []byte(`"inlineData"`)) {
		t.Fatalf("image not streamed immediately: %s %v", out, err)
	}
	if _, err := stream.Finish(); err == nil {
		t.Fatal("partial image stream fabricated terminal")
	}
}

func TestChatResponseRejectsInvalidImageParts(t *testing.T) {
	for _, block := range []string{`{"type":"image_url","image_url":{"url":"data:image/png;base64,%%%"}}`, `{"type":"image_url","image_url":{"url":"file:///private.png"}}`, `{"type":"image_url","image_url":{"url":"data:text/html;base64,aGk="}}`, `{"type":"image_url","image_url":{"url":"https://images.example/a.png","detail":"high"}}`, `{"type":"text","text":"ok","signature":"opaque"}`} {
		var content []any
		if err := json.Unmarshal([]byte("["+block+"]"), &content); err != nil {
			t.Fatal(err)
		}
		if _, err := chatResponseContentParts(content); err == nil {
			t.Fatalf("accepted nonportable image: %s", block)
		}
	}
}

func TestGeminiUsageKeepsIntegerArithmetic(t *testing.T) {
	usage, err := geminiUsage(map[string]any{"promptTokenCount": json.Number("9007199254740993"), "candidatesTokenCount": json.Number("2"), "thoughtsTokenCount": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(usage)
	if !bytes.Contains(out, []byte(`"total_tokens":9007199254740996`)) || !bytes.Contains(out, []byte(`"completion_tokens":3`)) {
		t.Fatalf("integer usage changed: %s", out)
	}
	zero, err := chatUsage(map[string]any{"prompt_tokens": json.Number("5"), "completion_tokens": json.Number("3"), "total_tokens": json.Number("0")})
	if err != nil || zero["totalTokenCount"] != int64(0) {
		t.Fatalf("explicit total overwritten: %v %v", zero, err)
	}
	for _, value := range []json.Number{"-1", "1.5", "9223372036854775808"} {
		if _, err := chatUsage(map[string]any{"prompt_tokens": value}); err == nil {
			t.Fatalf("invalid token count %s", value)
		}
	}
	if _, err := geminiUsage(map[string]any{"promptTokenCount": json.Number("9223372036854775807"), "candidatesTokenCount": json.Number("1")}); err == nil {
		t.Fatal("token sum overflow accepted")
	}
}

func TestGeminiResponseImagesRoundTripAndStream(t *testing.T) {
	raw := []byte(`{"responseId":"image-fixture","candidates":[{"content":{"role":"model","parts":[{"text":"first"},{"inlineData":{"mimeType":"image/png","data":"aGk="}},{"text":"last"},{"fileData":{"mimeType":"image/webp","fileUri":"https://images.example/output.webp?size=1"}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":9007199254740993,"candidatesTokenCount":2,"totalTokenCount":9007199254740995}}`)
	chat, err := ToChatResponse(raw, "image-model")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"content":[{"text":"first","type":"text"},{"image_url":{"url":"data:image/png;base64,aGk="},"type":"image_url"},{"text":"last","type":"text"}`, `"total_tokens":9007199254740995`} {
		if !bytes.Contains(chat, []byte(want)) {
			t.Fatalf("missing %s: %s", want, chat)
		}
	}
	back, err := FromChatResponse(chat)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := bridgeObject(raw)
	returned, _ := bridgeObject(back)
	parts := func(object map[string]any) []byte {
		candidate := object["candidates"].([]any)[0].(map[string]any)
		encoded, _ := json.Marshal(candidate["content"])
		return encoded
	}
	if !bytes.Equal(parts(original), parts(returned)) {
		t.Fatalf("image roundtrip changed: %s", back)
	}
	stream := NewGeminiStream("image-model")
	image := []byte("data: " + `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGk="}}]}}]}` + "\n\n")
	chunk, err := stream.TransformEvent(image)
	if err != nil || !bytes.Contains(chunk, []byte(`"image_url"`)) {
		t.Fatalf("image not emitted immediately: %s %v", chunk, err)
	}
	if _, err := stream.Finish(); err == nil {
		t.Fatal("partial image stream synthesized success")
	}
	complete := NewGeminiStream("image-model")
	chunks, err := complete.TransformEvent(append(append([]byte("data: "), raw...), []byte("\n\n")...))
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := complete.Finish()
	if err != nil || string(terminal) != "data: [DONE]\n\n" {
		t.Fatalf("wrong terminal %s %v", terminal, err)
	}
	reverse := NewChatStream("image-model")
	converted, err := reverse.TransformEvent(chunks)
	if err != nil || !bytes.Contains(converted, []byte(`"inlineData"`)) {
		t.Fatalf("stream image roundtrip: %s %v", converted, err)
	}
	if _, err := reverse.TransformEvent(terminal); err != nil {
		t.Fatal(err)
	}
}

func TestGeminiResponseRejectsLossyImageParts(t *testing.T) {
	credentialURL := (&url.URL{Scheme: "https", Host: "example.test", Path: "/img.png", User: url.UserPassword("user", "password")}).String()
	for _, part := range []string{
		`{"inlineData":{"mimeType":"image/png","data":"aGk="},"thoughtSignature":"signed"}`,
		`{"inlineData":{"mimeType":"image/png","data":"aGk="},"thought":true}`,
		`{"inlineData":{"mimeType":"audio/wav","data":"aGk="}}`,
		`{"inlineData":{"mimeType":"image/png","data":"%%%"}}`,
		`{"inlineData":{"mimeType":"image/png","data":"aGk=","secret":"unknown"}}`,
		`{"fileData":{"mimeType":"image/png","fileUri":"gs://provider/file"}}`,
		`{"fileData":{"mimeType":"image/png","fileUri":"` + credentialURL + `"}}`,
		`{"text":"one","inlineData":{"mimeType":"image/png","data":"aGk="}}`,
	} {
		raw := []byte(`{"candidates":[{"content":{"role":"model","parts":[` + part + `]},"finishReason":"STOP"}]}`)
		if _, err := ToChatResponse(raw, "m"); err == nil {
			t.Fatalf("accepted invalid image JSON: %s", raw)
		}
		stream := NewGeminiStream("m")
		if _, err := stream.TransformEvent(append(append([]byte("data: "), raw...), []byte("\n\n")...)); err == nil {
			t.Fatalf("accepted invalid image SSE: %s", raw)
		}
		if _, err := stream.Finish(); err == nil {
			t.Fatal("image SSE failure was repaired")
		}
	}
	if _, err := ToChatResponse([]byte(`{"candidates":[{"content":{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"aGk="}}]},"finishReason":"STOP"}]}`), "m"); err == nil {
		t.Fatal("response fabricated assistant role from a user message")
	}
}
