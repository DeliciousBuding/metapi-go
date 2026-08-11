# Contributing to MetAPI Go

Thanks for considering a contribution! This project follows a small, explicit
set of rules so the master branch stays releasable at all times.

## Development setup

- **Go 1.26+** for the backend (`go.mod` pins the exact version).
- **Bun 1.3+** for the frontend under `web/` (the SPA is pre-built and embedded
  into the Go binary via `go:embed web/dist`).
- No Node.js needed for backend work; the frontend build is Bun-only.

Install the local pre-push hook so every push runs the local CI gate first:

```bash
git config core.hooksPath .githooks
```

The hook runs `go build`, `go vet`, the frontend gate, and the race-enabled
test suite. Use `git push --no-verify` only for emergencies — GitHub Actions
still runs the full 11-check CI on every PR.

## Branch model

GitHub Flow — `master` is the only long-lived branch and is always releasable.
All work happens on short-lived branches:

| Prefix   | Purpose                      | Example                     |
|:---------|:-----------------------------|:----------------------------|
| `feature/` | New features               | `feature/model-tester`      |
| `fix/`     | Bug fixes                  | `fix/url-sync-nav`          |
| `perf/`    | Performance                | `perf/dashboard-lazy-load`  |
| `refactor/`| Refactoring, no behavior change | `refactor/search-params` |
| `chore/`   | Engineering (CI, deps, cleanup) | `chore/ci-hardening`     |
| `docs/`    | Documentation              | `docs/git-workflow`         |

Never commit directly to `master` — branch protection requires a PR.

## Local gates before pushing

```bash
go build ./cmd/server
go vet ./...
go test ./... -count=1 -race        # backend suite with race detector
cd web && bun install                # once
cd web && bun run typecheck && bun run lint && bun run test && bun run knip
cd web && bun run format:check       # oxfmt
make docs-hygiene                    # public-markdown hygiene (no local paths/secrets)
```

TL;DR: `make verify` covers the Go side; `make verify-race` adds the race
detector. The pre-push hook runs all of it automatically.

## Pull requests

1. Branch from `master`, commit with [Conventional Commits](https://www.conventionalcommits.org/).
2. Open a PR against `master` — the template is auto-filled; keep the summary
   about *why*, not a file-by-file list.
3. All 11 required CI checks must pass (they run on every PR, docs-only PRs
   included — no `paths-ignore` shortcuts).
4. PRs are **squash merged** (merge commits are disabled). The PR title becomes
   the commit message, so make it a good Conventional Commit line.

## Conventions to keep in mind

- **API contract**: JSON responses use camelCase fields; env var names are
  identical to the original TypeScript MetAPI (no prefix). Do not invent new
  env var names or break the wire format.
- **Dual dialect**: SQLite (dev/test) and PostgreSQL (production) are both
  supported — never write SQLite-only SQL. `store.Open(dialect, dsn)` is the
  entry point.
- **Frontend i18n**: all user-facing text goes through `t()` keys in
  `web/src/i18n/locales/{en,zh-CN}.json` — both languages must stay in sync
  (checked by `web/src/i18n/__tests__/i18n-keys.test.ts`).
- **Docs**: user-facing behavior changes update `docs/STATE.md` /
  `docs/progress/MASTER.md` / `CHANGELOG.md`. Public docs must never contain
  local paths, private hostnames, or credentials (`make docs-hygiene` enforces
  this).
- **Single binary**: the production image ships no Node/Bun runtime. Don't
  add npm scripts to the runtime path.

## Reporting issues & security

- Bug reports: use the issue templates (bug / feature request).
- Security vulnerabilities: **do not open a public issue** — report privately
  via GitHub Security Advisory, see [`SECURITY.md`](SECURITY.md).
- Questions: prefer opening a discussion over an issue.

## Code of conduct

By participating you agree to the [Contributor Covenant](CODE_OF_CONDUCT.md).

## License

MIT — by contributing you agree that your contributions are licensed under
the same terms as the repository.
