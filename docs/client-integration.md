# Client Integration

**Last updated**: 2026-10-30

Metapi exposes OpenAI-compatible endpoints under `/v1`, Anthropic Messages,
Responses and other protocol-specific paths. The usable features depend on
the selected route, upstream protocol and Key policy. This page describes
client configuration; a working text request does not validate every feature
of a client.

## Personal accounts

If an administrator gave you an ordinary account, activate it at `/user`,
open **Models**, and select a public model. The connection details show the
protocols for which the current configuration can provide an example. Create
a personal Key under **API keys**, save the secret once, and use the page's
environment-variable-based request example. Your login password is not a
downstream API Key.

Personal Keys follow your current group's access and share your account's
token quota. Bounded text, streaming and ordinary function-tool payloads
remain subject to the selected protocol and upstream's capabilities. Media,
hosted tools such as built-in web search, background work and opaque
continuations such as `previous_response_id` or `conversation` are not
available under the current personal quota policy, including unlimited
accounts. A listed Responses protocol does not promise that a client's
complete agent workflow will work. Legacy sources without protocol metadata
may list a model without a connection example.

After a request, open **Requests** to check the gateway outcome and recorded
quota settlement. Verify the actual result in your client as well. For
support, copy the internal request ID from the record; it is distinct from
the correlation header `X-Request-Id`. Rotating a Key preserves its identity,
limits and history but immediately invalidates the old secret for new
requests and the next turn of an existing WebSocket.

Search your retained requests by public model or internal request ID, then
filter by gateway result or quota settlement (including usage needing review).
The support summary copies only public request facts, not your Key or upstream
configuration. Filters remain when you close a request detail. A Key marked
enabled may still be expired or exhausted; the console shows those conditions
separately, while account quota and group permissions continue to apply.

## The two values every client needs

| Value      | Where to get it                                             |
| :--------- | :---------------------------------------------------------- |
| Base URL   | Gateway root `http://<host>:4000` or your public domain; OpenAI-compatible clients append `/v1`, while Claude Code uses the root. |
| API key    | A downstream key from **访问 → API 密钥** (`/downstream-keys`) |

Administrator-managed system Keys have optional request/cost caps and expiry.
Personal Keys additionally inherit their owner's current group and shared
quota. Create one per tool to manage access independently. Administrator
connection and config-export actions below require administrator access.

## Cursor

Settings → Models → enable **OpenAI API Key** override:

- API key: your downstream key
- Base URL (override): `http://<host>:4000/v1`

Use a public model name exposed by Metapi. The model page also displays upstream
supply names and catalog information, which need not be client-facing names.
Check `/v1/models` with the same downstream key you configure in Cursor.

## Claude Code

```bash
export ANTHROPIC_BASE_URL='http://<host>:4000'
export ANTHROPIC_AUTH_TOKEN='<downstream-key>'
```

Claude Code then speaks the Anthropic protocol; Metapi dispatches through a
route that can serve that protocol. Keep the base URL at the gateway root,
not `/v1`. The downstream Key's **连接 (Connect)** dialog exports a JSON
settings document, not a shell block:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://<host>:4000",
    "ANTHROPIC_AUTH_TOKEN": "<downstream-key>"
  }
}
```

Merge these values into your existing `~/.claude/settings.json`; do not replace
unrelated settings. Select a public model that the same Key can access.

## Codex CLI

```bash
export METAPI_API_KEY='<downstream-key>'
```

Merge the following into your **user-level** `~/.codex/config.toml`, replacing
the model with a public name returned by `/v1/models` for the same Key:

```toml
model_provider = "metapi"
model = "<public-model>"

[model_providers.metapi]
name = "MetAPI"
base_url = "http://<host>:4000/v1"
env_key = "METAPI_API_KEY"
wire_api = "responses"
```

The exported `codex` profile separates this TOML document from the shell step
that sets `METAPI_API_KEY`; the secret is not embedded in the provider table.
Merge top-level settings and the provider table without duplicating existing
keys or replacing unrelated configuration. Do not put provider configuration
in a repository's `.codex/config.toml`: Codex ignores provider and authentication
overrides there. See the official [custom-provider configuration](https://developers.openai.com/codex/config-advanced/).

The route must serve Responses for this Key; the personal Key restrictions above
still apply.

## Open WebUI

Admin → Settings → Connections → OpenAI API:

- Base URL: `http://<host>:4000/v1`
- API key: your downstream key

The model dropdown fills from the same Key's `/v1/models` catalog. It lists only
models currently eligible under that Key's permissions and routing checks.

## Cherry Studio / generic OpenAI clients

- API endpoint: `http://<host>:4000/v1`
- Key: downstream key
- Sync models from `/v1/models`.

## Config export from the admin UI

Select a downstream key → **连接 (Connect)**. After credential confirmation,
the dialog renders profiles for six targets — `openai`, `cherry`, `generic`,
`claude-code`, `codex`, `openwebui` — with client-specific addresses and env/file
layouts. The on-screen Key is masked until explicitly revealed; copying a
profile or confirming an external-app import can hand off the real Key.
Those actions are explicit and do not automatically overwrite the clipboard
after opening an application. Export credentials stay in the current dialog;
closing or switching Keys cancels the export and requires confirmation again.
Metapi does not rewrite your local configuration automatically. Review and merge
its model and protocol settings for the client version you use.

Exported configuration and the verification model are currently independent.
Selecting a model in the verification tab does not update a previously exported
client profile. Set the same public model in the client before interpreting a
verification result as evidence for that configuration.

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
attempt is logged. This proves only that one finite request in the selected
protocol returned valid assistant text; streaming, tools and the actual client
need separate acceptance.

For permission problems, use the key's permission check or the route workspace
with its client model and protocol. The administrator model tester uses a
different authorization context, so it cannot substitute for a check with the
client's own Key.

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
- `/v1/models` uses the same read-only eligibility checks as dispatch. It filters
  by Key permissions and candidate state without calling upstreams; it is not
  the unrestricted administrator catalog. See [model and route authorization](api/downstream-keys.md#模型与路由授权).
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
