# Responses WebSocket — C3 residual

**Status**: present (single-instance honesty)

The Responses WebSocket transport (`handler/proxy/responses_ws.go`) registers a
residual Codex upstream `wss` transport:

- `c3_codex_upstream_wss` status, enabled by capability probe (platform=codex +
  `CodexUpstreamWebsocketEnabled` + extraConfig)
- Codex upstream `wss` runtime + session response-id store
- Dial / empty-event failure falls back to the HTTP SSE bridge — no fake
  terminal frames
- **Process-local sticky only**: the transport is single-instance honest;
  there is no cluster-wide sticky session claim (no `STICKY-B`). Multi-instance
  deployments require load-balancer pinning or a single instance.

Forbidden always: hijack-silent-close and inventing terminal frames for failed
bridges.
