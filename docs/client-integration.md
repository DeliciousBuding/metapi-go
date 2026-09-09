# Client Integration

**Last updated**: 2026-08-20

Metapi exposes the standard OpenAI wire format on `/v1` (plus Claude native
`/v1/messages`, Responses, Embeddings, Images and `/v1/models`), so any tool
that speaks OpenAI works without plugins. This page covers the common
clients and the built-in config export.

## The two values every client needs

| Value      | Where to get it                                             |
| :--------- | :---------------------------------------------------------- |
| Base URL   | `http://<host>:4000/v1` (or your reverse-proxied domain)    |
| API key    | A downstream key from **设置 → 下游密钥**, or `PROXY_TOKEN`  |

Downstream keys are per-project keys with optional request/cost caps and
expiry — create one per tool instead of sharing `PROXY_TOKEN` everywhere.

## Cursor

Settings → Models → enable **OpenAI API Key** override:

- API key: your downstream key
- Base URL (override): `http://<host>:4000/v1`

Model names are whatever Metapi routes (see **模型广场** for the live list).

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

Select an account (or downstream key) → **导出凭证**. The dialog renders
ready-to-copy profiles for six targets — `openai`, `cherry`, `generic`,
`claude-code`, `codex`, `openwebui` — each with the correct base URL, key
placeholder and client-specific env/file layout, plus a **Send a test
request** shortcut into the model tester.

## Protocol notes

- SSE is supported on native Chat, Messages and Responses paths and on the
  supported Messages-to-Chat bridge described below.
- Native Chat, Messages and Responses requests prefer an upstream endpoint of
  the same protocol. A supported timeout/protocol failure may fall back from
  Messages to Chat Completions for representable text and client-function tools;
  both requests and JSON/SSE responses are translated, including tool-result turns.
- Other Chat/Messages/Responses conversion directions are not implemented and
  are not tried by forwarding an incompatible body. Endpoint preferences do not
  add a missing converter. A responses-only site rejects an incompatible native
  Chat/Messages request explicitly; use an upstream supporting the client protocol.
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
