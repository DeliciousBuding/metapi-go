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

- Streaming (SSE) is fully supported on all chat/completions/responses paths.
- OpenAI ⇄ Claude format conversion is automatic; pick the wire format your
  client speaks, not the upstream's.
- `/v1/models` aggregates every routed model; hidden or disabled channels are
  excluded honestly rather than advertised.
- Requests to models with no route return an explicit error, never a fake
  success.

See [`api.md`](api.md) for the full endpoint inventory and
[`getting-started.md`](getting-started.md) for the first-request walkthrough.
