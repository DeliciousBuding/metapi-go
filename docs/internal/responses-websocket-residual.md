# Responses WebSocket transport

The Responses WebSocket handler (`handler/proxy/responses_ws.go`) supports
Codex upstream `wss` and an HTTP SSE bridge:

- `c3_codex_upstream_wss` status, enabled by capability probe (platform=codex +
  `CodexUpstreamWebsocketEnabled` + extraConfig)
- Codex upstream `wss` runtime + session response-id store
- Dial / empty-event failure falls back to the HTTP SSE bridge.
- Sessions and response IDs are process-local. Multi-instance deployments
  require load-balancer pinning or a single instance.
- AxonHub's `openai/responses_websocket` source format is surfaced as an import
  residual. It does not add a direct-grant executor or imply transport support.

Terminal frames reflect upstream protocol events or an explicit bridge error.
