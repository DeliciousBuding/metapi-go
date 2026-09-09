// Package messages bridges the text and client-function-tool subset of Anthropic
// Messages to OpenAI Chat Completions. It owns no HTTP transport or global state.
//
// Requests fail closed on unrepresentable features, including images, signed thinking
// history, hosted tools, error-marked tool results, assistant prefills and nonempty stop
// sequences (Chat does not identify the matched sequence). Metadata and cache
// controls are intentionally not forwarded: they are attribution/cache hints,
// not transcript content. Unsupported fields are errors, not silently dropped.
//
// Adaptive thinking is mapped to reasoning_effort without disabling it. Only
// explicit no-op context-management edits are accepted. Exact thinking budgets
// and model-specific effort downgrades are not approximated.
//
// Hidden reasoning in tool responses requires caller-owned Options callbacks
// for replay in the next tool continuation. Adaptive tool history without a
// complete replay record fails closed. display:"omitted" hides reasoning from
// clients but does not disable computation or discard continuation state.
//
// Responses expose only final content and function calls. Chat reasoning_content
// is not a signed Anthropic thinking block and is never presented as final text.
// Token counts are populated only from upstream usage; absent counts remain
// absent, including in message_start. Reported cached prompt tokens are split
// from input_tokens; cache creation counts are never guessed.
//
// ChatStream accepts one complete SSE frame per TransformEvent call. Success
// requires both a finish_reason and an actual [DONE] frame. Finish validates EOF;
// it does not repair truncated tool JSON or synthesize a successful terminator.
package messages
