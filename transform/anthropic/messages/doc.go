// Package messages bridges Anthropic Messages and OpenAI Chat Completions.
// It owns no HTTP transport or global state.
//
// ToChatRequest fails closed on unrepresentable features, including images, signed thinking
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
//
// The reverse bridge uses FromChatRequest, ToChatResponse and MessagesStream.
// It supports leading system/developer instructions, text, image URLs/base64,
// client function tools and their results, sampling, stop sequences and cache
// hints. Missing Chat output limits become the Messages-required 4096-token
// limit. Signed thinking, continuity state and unsupported controls fail closed.
// Native cache reads and writes are included in Chat prompt totals and retained
// in prompt_tokens_details; absent usage counts are not guessed. A matched native
// stop sequence is retained in choice.stop_sequence. MessagesStream requires
// closed content blocks, valid tool argument objects and a real message_stop
// before emitting a Chat finish_reason and [DONE].
package messages
