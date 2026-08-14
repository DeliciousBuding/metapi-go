# Roadmap

**Last updated**: 2026-08-14

**Repo**: https://github.com/DeliciousBuding/metapi-go

**Release**: v0.11.0 · master CD `ghcr.io/deliciousbuding/metapi-go`

> Current status → [`../STATE.md`](../STATE.md) · Timeline → [`../log.md`](../log.md)

## Open items

| Issue | Title |
|------:|:------|
| [#557](https://github.com/DeliciousBuding/metapi-go/issues/557) | Production multi-channel cascade e2e |
| [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) | Runtime probes for Codex and AnyRouter (optional) |
| [#562](https://github.com/DeliciousBuding/metapi-go/issues/562) | 商汤端口兼容性（自动判失效） |

## Product honesty

- Sticky sessions: process-local only (single-instance or LB pinning required for multi-instance)
- WebSocket Responses: present (single-instance honesty)
- Update center: external (GHCR/releases); API residual remains 501
- Usage stats: present with residual (multi-instance aggregation is not exact billing)

## Pre-commit checks

```bash
go build ./cmd/server && go vet ./... && go test ./... -count=1 -race
```
