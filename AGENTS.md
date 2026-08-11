# MetAPI Go — Engineering Rules

Go rewrite of [MetAPI](https://github.com/cita-777/metapi). Feature parity with the original TypeScript version.

## Golden Rules

- **Feature parity**: Every behavior must match the original TypeScript MetAPI server.
  Keep the TS reference checkout outside this public repo, and do not document local checkout paths.
- **Single binary**: The React SPA is pre-built and embedded via `go:embed`. Do not add `npm`/`node` to the
  production image.
- **Dual dialect**: SQLite (dev/test) and PostgreSQL (production). Use `store.Open(dialect, dsn)`. Never
  assume SQLite-only features.
- **API compatibility**: All JSON responses must use camelCase field names matching the TS frontend.
  All env var names are identical to the TS version (no prefix).
- **Before pushing**: `go build ./cmd/server && go vet ./... && go test ./... -count=1 -race` must pass.
  🚫 **Never skip local CI** — GitHub Actions is a verification gate, not a debug environment. The pre-push hook enforces this.

## Project Structure

```
cmd/server/main.go      Entry point
cmd/migrate/main.go     SQLite→PG migration tool
config/                 ~100 env vars from config.Load()
store/                  DB layer (28 tables, sqlx)
auth/                   Admin + proxy auth + rate limiting
routing/                TokenRouter (Fibonacci + weighted random)
proxy/                  ProxyCore (dual-loop orchestration)
platform/               14 upstream adapters
transform/              4-protocol SSE conversion
service/                Checkin, balance, notify, OAuth, backup
scheduler/              16 background jobs
handler/admin/          ~144 admin REST endpoints
handler/proxy/           ~30 proxy routes (OpenAI, Gemini, Claude, Codex, Files)
web/dist/               Pre-built React SPA (embedded)
```

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/jmoiron/sqlx` | DB access |
| `modernc.org/sqlite` | Pure-Go SQLite (no CGO) |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/robfig/cron/v3` | Cron scheduler |

## Build & Test

```bash
go build -o metapi ./cmd/server       # Build server
go build -o metapi-migrate ./cmd/migrate  # Build migration tool
go test ./... -count=1 -race          # Run all tests with race detector
go vet ./...                          # Static analysis
golangci-lint run --timeout=3m        # Lint check
```

## Release Workflow

0. 所有改动经 `fix/*` / `feature/*` 等短命分支 → PR → Squash merge 回 master（详见 [`docs/git-workflow.md`](docs/git-workflow.md)；master 受保护，禁止直接 push）
1. 确保本地 CI 全部通过（pre-push hook 自动检查）
2. 更新 `CHANGELOG.md`（按 Keep a Changelog 格式）
3. Tag + push：`git tag -a vX.Y.Z -m "vX.Y.Z — 简述"` → `git push origin vX.Y.Z`
4. Tag push 触发 GitHub Actions `release.yml` → 自动创建 GitHub Release
5. CD 自动构建 Docker 镜像推送到 `ghcr.io/deliciousbuding/metapi-go:vX.Y.Z`

**版本号**：`vMAJOR.MINOR.PATCH`（SemVer 2.0）
- PATCH：bug 修复
- MINOR：新功能/性能优化
- MAJOR：不兼容 API 变更
- v0.x 阶段 minor 可用于新功能

## CI Discipline

- **Do not push if local CI fails**: all pushes must pass `go vet ./... && go test ./... -count=1 -race` first
- The git pre-push hook (`.githooks/pre-push`) automatically blocks pushes that fail local CI
- Emergency skip: `git push --no-verify`
- GitHub Actions is the final verification gate, not a debug environment

## Specs & Docs

**Map (start here):** [`docs/README.md`](docs/README.md)

| Path | Role |
|------|------|
| `docs/STATE.md` | **Current state** (verified product facts; keep slim) |
| `docs/progress/MASTER.md` | **Open items + strict checks** (not a changelog) |
| `docs/log.md` | **Progress log** append-only (never overrides STATE) |

| `docs/architecture.md` | As-built package map (proxy/transform/routing; not proxycore/protocol) |
| `docs/design/BACKEND.md` | Backend philosophy, dependency rules, forbidden imports |
| `docs/design/DESIGN.md` | UI design system source of truth |

| `docs/api.md` / `docs/deployment.md` / `docs/migration.md` | API · deploy · migration |
| `CHANGELOG.md` | Version narrative |

**Progress roles:** STATE = current state · MASTER = open items · LOG = timeline. Temporary session summaries are **not** source of truth — archive or delete after use.
**Ops host/image pin** lives outside this repository (private deployment surface). Public deployment notes: `docs/deployment.md`.
**Honesty:** Prefer 501 / documented residual over stub theater. Do not claim cluster-wide sticky or WS product without the matching milestone.

## Related References

- TS source: original TypeScript MetAPI repository, checked out separately when parity work needs it.
- Gateway fork: private deployment, not part of this public repo.
- Ops skill: operator-local reference only. Do not publish private filesystem paths.
