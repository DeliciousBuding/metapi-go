package proxyhandler

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const nanoGPTDeclaredRequest = `{"model":"provider-model","messages":[{"role":"user","content":"use tools"}],"tools":[{"type":"function","function":{"name":"Read","parameters":{"type":"object"}}},{"type":"function","function":{"name":"Write_File","parameters":{"type":"object"}}},{"type":"function","function":{"name":"namespace__lookup","parameters":{"type":"object"}}}]}`

func nanoGPTTestTools(t *testing.T) *directNanoGPTTools {
	t.Helper()
	tools, err := newDirectNanoGPTTools([]byte(nanoGPTDeclaredRequest))
	if err != nil {
		t.Fatal(err)
	}
	return tools
}

func TestDirectNanoGPTRequestReasoningAndToolDeclaration(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"assistant","reasoning_content":"keep thinking","tool_calls":[{"id":"native-id","type":"function","function":{"name":"Read","arguments":"{\"id\":9007199254740993}"}}]}],"large":9007199254740993}`)
	out, err := prepareDirectNanoGPTRequest(body)
	if err != nil || !bytes.Contains(out, []byte(`"reasoning":"keep thinking"`)) || bytes.Contains(out, []byte(`"reasoning_content"`)) || !bytes.Contains(out, []byte("9007199254740993")) || !bytes.Contains(out, []byte("native-id")) {
		t.Fatalf("request semantics changed: %s %v", out, err)
	}
	if _, err := prepareDirectNanoGPTRequest([]byte(`{"messages":[{"reasoning":"a","reasoning_content":"b"}]}`)); err == nil {
		t.Fatal("conflicting reasoning was silently overwritten")
	}
	for _, request := range []string{`{"messages":[]}`, strings.TrimSuffix(nanoGPTDeclaredRequest, "}") + `,"tool_choice":"none"}`, strings.TrimSuffix(nanoGPTDeclaredRequest, "}") + `,"tool_choice":"n\u006fne"}`} {
		tools, err := newDirectNanoGPTTools([]byte(request))
		if err != nil {
			t.Fatal(err)
		}
		text, calls, err := newNanoGPTText(tools).feed(`<Read file_path="x"/>`, true)
		if err != nil || len(calls) != 0 || text != `<Read file_path="x"/>` {
			t.Fatal("XML inferred undeclared/disabled tools")
		}
	}
	plainTools := nanoGPTTestTools(t)
	streamTools, err := newDirectNanoGPTTools([]byte(strings.TrimSuffix(nanoGPTDeclaredRequest, "}") + `,"stream":true,"stream_options":{"include_usage":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	call := nanoGPTXMLCall{name: "Read", arguments: `{"x":1}`}
	if nanoGPTCallID(plainTools, "", 0, 0, call) != nanoGPTCallID(streamTools, "", 0, 0, call) {
		t.Fatal("stream mode changed fallback XML identity")
	}
}

func TestDirectNanoGPTXMLFormsAcrossEverySplit(t *testing.T) {
	for _, tc := range []struct{ text, remaining, name, args string }{
		{`<Read file_path="a&amp;b"/>`, "", "Read", `{"file_path":"a&b"}`},
		{`<Write_File>{"file_path":"x","count":9007199254740993}</Write_File>`, "", "Write_File", `{"count":9007199254740993,"file_path":"x"}`},
		{"before\n<Write_File file_path=\"x\">  exact text  </Write_File>\nafter", "before\n\nafter", "Write_File", `{"content":"  exact text  ","file_path":"x"}`},
		{"<use_tool name=\"namespace__lookup\">\n<parameter name=\"query\">a &lt; b</parameter>\n</use_tool>", "", "namespace__lookup", `{"query":"a < b"}`},
		{`<Write_File><file_path>x</file_path><content><![CDATA[<Read file_path="example"/>]]></content></Write_File>`, "", "Write_File", `{"content":"<Read file_path=\"example\"/>","file_path":"x"}`},
	} {
		for split := 0; split <= len(tc.text); split++ {
			p := newNanoGPTText(nanoGPTTestTools(t))
			left, first, err := p.feed(tc.text[:split], false)
			if err != nil {
				t.Fatal(err)
			}
			right, second, err := p.feed(tc.text[split:], true)
			if err != nil {
				t.Fatalf("split=%d text=%s err=%v", split, tc.text, err)
			}
			calls := append(first, second...)
			if left+right != tc.remaining || len(calls) != 1 || calls[0].name != tc.name || !nanoGPTJSONEqual(calls[0].arguments, tc.args) {
				t.Fatalf("split=%d remaining=%q calls=%+v", split, left+right, calls)
			}
		}
	}
}

func nanoGPTJSONEqual(a, b string) bool {
	decode := func(raw string) any {
		var value any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		_ = decoder.Decode(&value)
		return value
	}
	return reflect.DeepEqual(decode(a), decode(b))
}

func TestDirectNanoGPTXMLExamplesUnknownAndMalformed(t *testing.T) {
	for _, text := range []string{
		"```xml\n<Read file_path=\"example\"/>\n```\n", "~~~xml\n<Read/>\n~~~\n", "`<Read/>`", "``<Read/>``", "    <Read/>", "> <Read/>",
		"`multiline example\n<Read/>\n`", "Text with ``multiline\n<Read/>\ncode``.",
		`Example: <Read/>`, `<Read/> is an example`, `<read file_path="wrong-case"/>`, `<Read_file file_path="not-declared"/>`, `<use_tool name="read"/>`,
		"<document>\n<Read/>\n</document>", "<document>\n<Read/>\n", "<!--\n<Read/>\n-->", "<![CDATA[<Read/>]]>",
		`<Write_File file_path="x"`, "<Read>\n<file_path>x</file_path>", `<div>unknown</div>`, `<Read invalid>text</Read> is an example`,
	} {
		p := newNanoGPTText(nanoGPTTestTools(t))
		var got strings.Builder
		var calls []nanoGPTXMLCall
		for i := 0; i < len(text); i++ {
			out, next, err := p.feed(text[i:i+1], false)
			if err != nil {
				t.Fatalf("example errored: %q %v", text, err)
			}
			got.WriteString(out)
			calls = append(calls, next...)
		}
		out, next, err := p.feed("", true)
		got.WriteString(out)
		calls = append(calls, next...)
		if err != nil || len(calls) != 0 || got.String() != text {
			t.Fatalf("example became an action or changed: input=%q output=%q calls=%+v err=%v", text, got.String(), calls, err)
		}
	}
	for _, malformed := range []string{`<Read>x</use_tool>`, `<Read x="1" x="2"/>`, `<Read><path>a</path><path>b</path></Read>`, `<Read path="a">{"path":"b"}</Read>`} {
		if _, _, err := newNanoGPTText(nanoGPTTestTools(t)).feed(malformed, true); err == nil {
			t.Fatalf("ambiguous tool XML was repaired: %s", malformed)
		}
	}
}

func nanoGPTTestResponse(content string) []byte {
	encoded, _ := json.Marshal(map[string]any{"id": "nano-response", "object": "chat.completion", "model": "provider-model", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content, "reasoning": "supplied thinking", "tool_calls": []any{map[string]any{"id": "native-id", "type": "function", "function": map[string]string{"name": "ExistingCase", "arguments": "{}"}}}}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}})
	return encoded
}

func TestDirectNanoGPTJSONKeepsNativeCallsAndDistinctXMLIdentities(t *testing.T) {
	text := "before\n<Read path=\"same\"/><Read path=\"same\"/>\n<Unknown><Read/></Unknown>\nafter"
	out, err := normalizeDirectNanoGPTJSON(nanoGPTTestResponse(text), nanoGPTTestTools(t))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning_content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.Unmarshal(out, &response) != nil {
		t.Fatal("invalid normalized JSON")
	}
	choice := response.Choices[0]
	calls := choice.Message.ToolCalls
	if len(calls) != 3 || calls[0].ID != "native-id" || calls[0].Function.Name != "ExistingCase" || calls[1].ID == calls[2].ID || calls[1].Function.Name != "Read" || choice.Message.Content != "before\n\n<Unknown><Read/></Unknown>\nafter" || choice.Message.Reasoning != "supplied thinking" || choice.Finish != "tool_calls" {
		t.Fatalf("lost response semantics: %s", out)
	}
	if bytes.Contains(out, []byte(`"reasoning":`)) {
		t.Fatal("unconsumed reasoning alias leaked to downstream converters")
	}
}
