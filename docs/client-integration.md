# Client Integration

**Last updated**: 2026-10-15

Metapi exposes the standard OpenAI wire format on `/v1` (plus Claude native
`/v1/messages`, Responses, Embeddings, Images and `/v1/models`), so any tool
that speaks OpenAI works without plugins. This page covers the common
clients and the built-in config export.

## The two values every client needs

| Value      | Where to get it                                             |
| :--------- | :---------------------------------------------------------- |
| Base URL   | `http://<host>:4000/v1` (or your reverse-proxied domain)    |
| API key    | A downstream key from **访问 → API 密钥** (`/downstream-keys`) |

Downstream keys are per-project keys with optional request/cost caps and
expiry. Create one per tool to manage access independently.

## Cursor

Settings → Models → enable **OpenAI API Key** override:

- API key: your downstream key
- Base URL (override): `http://<host>:4000/v1`

Use a public model name exposed by Metapi. The model page also displays upstream
supply names and catalog information, which need not be client-facing names.
Check `/v1/models` with the same downstream key you configure in Cursor.

## Claude Code

```bash
export ANTHROPIC_BASE_URL=http://<host>:4000
export ANTHROPIC_AUTH_TOKEN=<downstream-key>
```

Claude Code then speaks the Anthropic protocol; Metapi translates to the
best available upstream channel. The admin UI's credential export dialog
(**账号 → 导出**) produces this env block ready to paste into
`~/.claude/settings.json`.

## Codex CLI

```bash
export OPENAI_BASE_URL=http://<host>:4000/v1
export OPENAI_API_KEY=<downstream-key>
```

## Open WebUI

Admin → Settings → Connections → OpenAI API:

- Base URL: `http://<host>:4000/v1`
- API key: your downstream key

The model dropdown fills from `/v1/models`, so every routed model appears
automatically.

## Cherry Studio / generic OpenAI clients

- API endpoint: `http://<host>:4000/v1`
- Key: downstream key
- Sync models from `/v1/models`.

## Config export from the admin UI

Select a downstream key → **连接 (Connect)**. After credential confirmation,
the dialog renders
ready-to-copy profiles for six targets — `openai`, `cherry`, `generic`,
`claude-code`, `codex`, `openwebui` — each with the correct base URL, key
placeholder and client-specific env/file layout. Copying a profile is explicit;
Metapi does not rewrite your local configuration automatically. Review its model
and protocol settings for the client version you use.

The connection dialog has two checks: list models with the key, then send one
finite non-streaming request for the selected model and an explicitly selected
protocol. The choices use the gateway's native data-plane shapes:

- Chat Completions calls `/v1/chat/completions` with `messages` and
  `max_tokens`.
- Responses calls `/v1/responses` with `input` and `max_output_tokens`.
- Anthropic Messages calls `/v1/messages` with `messages` and `max_tokens`, plus
  the `x-api-key` and `anthropic-version` headers.

The generation consumes quota and its attempt is logged. A verification passes
only when the selected protocol returns nonempty assistant text in its expected
response envelope; HTTP 200 with an error, empty content, or a malformed shape
fails. The dialog does not retry or switch protocols after a failure, because
that could consume quota twice. The key-filtered log link shows recorded
dispatch attempts; an authentication or admission refusal may happen before an
attempt is logged.

For permission problems, use the key's permission check or the route workspace
with its client model and protocol. The administrator model tester uses a
different authorization context. Verify the chosen client itself for streaming,
tools and protocol-specific behavior. A passing connection check proves only
that one finite request in the selected protocol returned valid assistant text;
it does not accept the full client, streaming, or tool use.

## Protocol notes

- Native Chat, Messages, Responses and Gemini generation paths support streaming.
  Independent upstreams with explicit endpoint configuration can convert among
  these four generation protocols when the route item authorizes the outbound
  protocol. Account-derived channels and legacy grants retain their own support
  boundaries; see [route and endpoint contracts](api/routes.md).
- Selection prefers the authorized native protocol. Within a direct channel,
  a different authorized protocol is tried only after an explicit unsupported
  endpoint/model response without output or measured usage. Authentication,
  throttling, transport failures and interrupted streams do not trigger this
  cross-protocol replay.
- Conversions carry representable text, tools and reported usage. Protocol-specific
  continuity, signed reasoning or unsupported tools fail explicitly. Non-generation
  endpoints such as embeddings and count-tokens require their own authorization
  and compatible endpoint; they are not replaced by a chat request.
- Messages features without a lossless Chat representation stay native-only.
  Metadata and prompt-cache hints are not forwarded through the Chat bridge;
  billing still uses the usage actually reported by the upstream.
- Adaptive thinking can map to Chat `reasoning_effort` (default `high`;
  explicit `low`, `medium`, `high` and `xhigh` are preserved). Exact thinking
  budgets, signed/redacted thinking blocks, images, hosted tools, assistant
  prefills, nonempty stop sequences, error tool results and context edits other
  than retaining all thinking remain native-only.
- A bridge tool reply may carry a `toolu_mcb_` ID. Keep that ID and the complete
  ordered history unchanged. Continuation requires the same authenticated key,
  session identity (or `metadata.user_id`), requested model and authorized
  upstream credential. Metapi selects the original channel through the normal
  routing-policy check; disabling the fallback or changing/revoking that
  credential does not authorize sending a lossy native transcript instead.
- Hidden tool reasoning is kept off client-visible content in a process-local
  cache: 256 tool groups, 8 MiB total, 512 KiB per group, 30-minute expiry.
  Eviction, expiry, restart or a different instance can make continuation fail
  explicitly; start a new conversation. This is not cross-instance stickiness
  or durable conversation storage. Stateless text/tool calls without a replay
  identity remain possible only when no hidden tool reasoning must be retained.
- `/v1/models` aggregates every routed model; hidden or disabled channels are
  excluded honestly rather than advertised.
- Requests to models with no route return an explicit error, never a fake
  success.

See [`api.md`](api.md) for the full endpoint inventory and
[`getting-started.md`](getting-started.md) for the first-request walkthrough.

## Native tool response compatibility

For OpenAI Chat Completions and Anthropic Messages, complete native tool calls
are returned with their protocol-specific tool termination reason. A provider's
ordinary `stop` or `end_turn` is corrected only when all observed tool arguments
form complete JSON objects; streamed tool blocks must also be closed. Empty
Chat streaming finish reasons become `null` on non-terminal chunks.

This does not turn errors, token-limit endings, refusals, malformed arguments,
or interrupted streams into success. Reasoning/signatures, tool identifiers,
arguments, usage and provider extension fields remain intact. Non-chat APIs,
attachments and undecodable representations are not rewritten.
