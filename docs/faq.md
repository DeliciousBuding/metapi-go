# FAQ

**Last updated**: 2026-08-20

Frequently asked questions. Anything not covered here lives in
[`configuration.md`](configuration.md) (env vars), [`deployment.md`](deployment.md)
(ops) and [`migration.md`](migration.md) (moving to Metapi Go).

## General

**What is Metapi, in one sentence?**
A self-hosted meta-aggregation gateway that unifies the AI relay sites you
already registered (New API / One API / OneHub / Sub2API / …) behind one API
key, with automatic model discovery, cost-aware routing and failover.

**How is this different from the TypeScript version?**
It is a ground-up Go rewrite of
[cita-777/metapi](https://github.com/cita-777/metapi) with client-visible
compatibility: same env var names, same JSON field casing, same wire
behavior — but a single ~15 MB binary, ~20 MB memory and instant startup.
See [migration.md](migration.md) to move an existing deployment.

**Is MySQL supported?**
No. Runtime databases are SQLite (single node, zero-config) and PostgreSQL
(production). The `metapi-migrate` tool transfers data between SQLite and
PostgreSQL only (either direction, plus a SQLite→SQLite copy) — it cannot
read a MySQL source. If your TypeScript deployment runs on MySQL, convert
it from inside the TS admin UI first (Settings → Database migration: pick
SQLite or PostgreSQL as the target, test the connection, keep the overwrite
box checked, run it), then take over the resulting database with Metapi Go.
Full walkthrough in [migration.md](migration.md), scenario C.

**Where is my data stored?**
Entirely in your own deployment (`DATA_DIR`, default `./data`). Metapi never
phones home; proxy traffic flows only between your server and your upstream
sites.

**Can I run multiple instances?**
Yes, with PostgreSQL. Side-effecting schedulers coordinate via PG advisory
locks; an optional Redis (`REDIS_URL`) shares downstream-key RPM/TPM
admission counters across instances. The following remain per-instance: sticky
sessions and realtime WebSocket panels — put a load balancer in front with
session pinning if you need stickiness.

## Deployment

**What are the defaults?**
Port `4000`; admin token `AUTH_TOKEN`, proxy key `PROXY_TOKEN` (change both);
SQLite at `DATA_DIR/hub.db`. On Windows, empty `HOST` binds `127.0.0.1` to
avoid firewall prompts — set `HOST=0.0.0.0` for LAN access.

**How do I upgrade?**
`docker compose pull && docker compose up -d` (or replace the binary). Schema
upgrades are additive and run automatically at startup; keep a backup of
`./data` before major version bumps like any database.

**Why does the admin UI not load behind my reverse proxy?**
WebSocket endpoints (realtime panel, Responses WS) need upgrade headers.
The nginx/Caddy templates in [deployment.md](deployment.md) include them.

**Why does a proxy request return 503 instead of a fake answer?**
Unconfigured forwarding is reported honestly: no route/channel for the
model, or no upstream configured. Configure a site → account → route and the
request flows. Auto-rebuild is not a substitute for that last step: it
recomposes the channels of routes you already have and never creates one, so on
a fresh install it finishes with `routesConsidered: 0` and the 503 stays. Test/demo stubbing exists behind
`METAPI_ENABLE_PROXY_STUB` and is never on by default.

## Routing & models

**How does the router choose a channel?**
Probability-weighted across enabled channels by cost, balance and usage,
with four-level cost truth (measured → configured → models.dev catalog →
fallback). Failed channels cool down and re-enter via half-open probing.

**Do new upstream models appear automatically?**
Discovery does: model lists refresh per site (on login, on demand, and on the
scheduled sync), and every refresh closes with a route rebuild, so a newly
discovered model gains a channel on any route whose pattern already covers it.
A model no existing pattern matches still needs a route of its own — the rebuild
recomposes channels, it does not create routes.

**What does the price column mean for relay sites?**
Prices joined from a concrete model + account are labeled with their source.
Catalog reference prices for third-party relays are always marked
`catalog_estimate` — they are never presented as your real payment price.

## Accounts & credentials

**Are my upstream tokens stored safely?**
Encrypted at rest (AES-GCM keyed by `ACCOUNT_CREDENTIAL_SECRET`, which falls
back to `AUTH_TOKEN` if unset — set a dedicated secret in production).

**What happens when a session token expires?**
Platforms with login support re-authenticate automatically; OAuth-connected
accounts refresh via their refresh token. Accounts that cannot renew are
marked unhealthy and alerted, not silently used.

## Contributing & support

**Where do I report a bug or security issue?**
GitHub issues for defects; [SECURITY.md](../SECURITY.md) for responsible
disclosure. Sanitized request/response shapes and versions help enormously;
never attach real credentials or `.env` files.

**Can I contribute?**
Yes — see [CONTRIBUTING.md](../CONTRIBUTING.md). Every change lands through a
short-lived branch + PR with the 12-check CI green.
