package relaykitbridge

var formats = []Format{Chat, Responses, Messages, Gemini}
var requests = map[Format]string{
	Chat:      `{"model":"fixture-model","max_tokens":512,"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"id":{"type":"integer"}}}}}]}`,
	Responses: `{"model":"fixture-model","max_output_tokens":512,"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"id":{"type":"integer"}}}}]}`,
	Messages:  `{"model":"fixture-model","max_tokens":512,"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"id":{"type":"integer"}}}}]}`,
	Gemini:    `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":512},"tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"object","properties":{"id":{"type":"integer"}}}}]}]}`,
}
var replies = map[Format]string{
	Chat:      `{"id":"chat_fixture","object":"chat.completion","model":"fixture-model","created":1700000000,"choices":[{"index":0,"message":{"role":"assistant","content":"hello","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"prompt_tokens_details":{"cached_tokens":4}}}`,
	Responses: `{"id":"resp_fixture","object":"response","model":"fixture-model","created_at":1700000000,"status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"id\":9007199254740993}","status":"completed"}],"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4}}}`,
	Messages:  `{"id":"msg_fixture","type":"message","model":"fixture-model","role":"assistant","content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"call_1","name":"lookup","input":{"id":9007199254740993}}],"stop_reason":"tool_use","usage":{"input_tokens":12,"output_tokens":7,"cache_read_input_tokens":4,"cache_creation_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}}}`,
	Gemini:    `{"responseId":"gem_fixture","modelVersion":"fixture-model","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hello"},{"functionCall":{"id":"call_1","name":"lookup","args":{"id":9007199254740993}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7,"totalTokenCount":19,"cachedContentTokenCount":4}}`,
}
var streams = map[Format][]string{
	Chat:      {`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture-model","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"},"finish_reason":null}]}`, `{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`},
	Responses: {`{"type":"response.created","response":{"id":"resp_fixture","model":"fixture-model","status":"in_progress","output":[]}}`, `{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"hello"}`, `{"type":"response.completed","response":{"id":"resp_fixture","model":"fixture-model","status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19}}}`},
	Messages:  {`{"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","model":"fixture-model","content":[],"usage":{"input_tokens":12,"output_tokens":1}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`, `{"type":"message_stop"}`},
	Gemini:    {`{"responseId":"gem_fixture","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"hello"}]}}]}`, `{"responseId":"gem_fixture","candidates":[{"index":0,"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7,"totalTokenCount":19}}`},
}

type specialFixture struct {
	Name       string
	Format     Format
	Phase, Raw string
}

var specials = []specialFixture{
	{"trailing-assistant", Chat, "request", `{"model":"fixture-model","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"prefill"}]}`},
	{"claude-cache-request", Messages, "request", `{"model":"fixture-model","max_tokens":512,"system":[{"type":"text","text":"cached instruction","cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":"hello"}]}`},
	{"unknown-request-field", Chat, "request", `{"model":"fixture-model","messages":[{"role":"user","content":"hello"}],"future_policy":{"must_preserve":true}}`},
	{"unknown-response-field", Messages, "response", `{"id":"m","type":"message","role":"assistant","model":"fixture-model","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","future_policy":{"must_preserve":true},"usage":{"input_tokens":1,"output_tokens":1}}`},
	{"hosted-tool-loss", Gemini, "request", `{"contents":[{"role":"user","parts":[{"text":"run code"}]}],"tools":[{"codeExecution":{}}]}`},
	{"opaque-thinking-request", Messages, "request", `{"model":"fixture-model","max_tokens":512,"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"thinking","thinking":"hidden fixture reasoning","signature":"opaque_signature"},{"type":"tool_use","id":"call_1","name":"lookup","input":{"id":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"done"}]}]}`},
	{"encrypted-reasoning-request", Responses, "request", `{"model":"fixture-model","input":[{"type":"reasoning","summary":[],"encrypted_content":"opaque_encrypted"},{"role":"user","content":"continue"}]}`},
	{"opaque-thinking-response", Messages, "response", `{"id":"m","type":"message","role":"assistant","model":"fixture-model","content":[{"type":"redacted_thinking","data":"opaque_redacted"},{"type":"text","text":"hello"}],"stop_reason":"pause_turn","usage":{"input_tokens":12,"output_tokens":7}}`},
	{"image-inline", Responses, "request", `{"model":"fixture-model","input":[{"role":"user","content":[{"type":"input_text","text":"before"},{"type":"input_image","image_url":"data:image/png;base64,AQID","detail":"original"},{"type":"input_text","text":"after"}]}]}`},
	{"image-url", Chat, "request", `{"model":"fixture-model","messages":[{"role":"user","content":[{"type":"text","text":"before"},{"type":"image_url","image_url":{"url":"https://images.example/fixture.png","detail":"high"}},{"type":"text","text":"after"}]}]}`},
	{"image-file-id", Responses, "request", `{"model":"fixture-model","input":[{"role":"user","content":[{"type":"input_image","file_id":"file_fixture"}]}]}`},
	{"schema-big-integer", Responses, "request", `{"model":"fixture-model","input":"hello","text":{"format":{"type":"json_schema","name":"result","strict":true,"schema":{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993}},"required":["id"],"additionalProperties":false}}}}`},
	{"image-output", Gemini, "response", `{"candidates":[{"content":{"role":"model","parts":[{"text":"before"},{"inlineData":{"mimeType":"image/png","data":"AQID"}},{"text":"after"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7,"totalTokenCount":19}}`},
}

var toolStreams = map[Format][]string{
	Chat: {
		`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"id\":"}}]},"finish_reason":null}]}`,
		`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"9007199254740993}"}}]},"finish_reason":null}]}`,
		`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`,
	},
	Responses: {
		`{"type":"response.created","response":{"id":"resp_fixture","model":"fixture-model","status":"in_progress","output":[]}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"","status":"in_progress"}}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"id\":"}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"9007199254740993}"}`,
		`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"{\"id\":9007199254740993}"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"id\":9007199254740993}","status":"completed"}}`,
		`{"type":"response.completed","response":{"id":"resp_fixture","model":"fixture-model","status":"completed","output":[{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"id\":9007199254740993}","status":"completed"}],"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19}}}`,
	},
	Messages: {
		`{"type":"message_start","message":{"id":"msg_fixture","type":"message","role":"assistant","model":"fixture-model","content":[],"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":4,"cache_creation_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"id\":"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"9007199254740993}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	},
	Gemini: {
		`{"responseId":"gem_fixture","candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{"id":9007199254740993}}}]}}]}`,
		`{"responseId":"gem_fixture","candidates":[{"index":0,"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7,"totalTokenCount":19}}`,
	},
}

func init() {
	streams[Responses] = []string{
		`{"type":"response.created","response":{"id":"resp_fixture","object":"response","model":"fixture-model","status":"in_progress","output":[]}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
		`{"type":"response.content_part.added","output_index":0,"content_index":0,"item_id":"msg_1","part":{"type":"output_text","text":"","annotations":[]}}`,
		`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"hello"}`,
		`{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"hello"}`,
		`{"type":"response.content_part.done","item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"hello","annotations":[]}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}}`,
		`{"type":"response.completed","response":{"id":"resp_fixture","object":"response","model":"fixture-model","status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}],"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19}}}`,
	}
}
