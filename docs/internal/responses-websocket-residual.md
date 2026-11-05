# Responses WebSocket transport

The Responses WebSocket handler (`handler/proxy/responses_ws.go`) supports
Codex upstream `wss` and an HTTP SSE bridge:

- `c3_codex_upstream_wss` status, enabled by capability probe (platform=codex +
  `CodexUpstreamWebsocketEnabled` + extraConfig)
- Codex upstream `wss` runtime + session response-id store
- Site Codex dial / empty-event failure retains its existing HTTP SSE bridge.
- Sessions and response IDs are process-local. Multi-instance deployments
  require load-balancer pinning or a single instance.
- Direct upstreams use an explicit `responsesWebsocket` endpoint and grant.
  Native API-key and Codex OAuth profiles share this runtime, with fresh
  authorization and credential resolution every turn. HTTP and WS permissions
  remain distinct.
- Downstream turns execute serially with at most one pending client frame.
  Disconnect cancels an in-flight queue wait or socket read. A burst exceeding
  that pending-frame bound closes the connection rather than accumulating work.
- A direct socket's identity includes the downstream Key and credential
  generation. Rotation, endpoint/proxy changes, disconnect and idle expiry
  prevent reuse of an old connection. State is scoped to the downstream socket.
- Direct fallback is allowed only after an upgrade failure before a generation
  event was sent, through a separately authorized HTTP grant. A write or read
  failure after sending an event never replays it, including automatic
  `previous_response_id` recovery.
- Native `response.incomplete` terminals for token limits/content filtering
  preserve their body and usage; Codex retains its completed-only contract.
  Failed terminal frames retain measured usage and a failed attempt outcome.

Terminal frames reflect upstream protocol events or an explicit bridge error.
