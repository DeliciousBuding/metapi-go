#!/usr/bin/env bash
# Fast local proof of the current change; --full remains a separate explicit gate.
set -euo pipefail
export GOFLAGS=" "
export GOMAXPROCS="${GOMAXPROCS:-2}"
if [[ $# != 0 && $# != 2 ]]; then echo 'verify-change: expected Git remote name/URL or no arguments' >&2; exit 2; fi
delete_only=0
nondelete=0
branch_push=0
tag_commits=()
while read -r local_ref local_oid _remote_ref _remote_oid; do
  if [[ "$local_oid" == 0000000000000000000000000000000000000000 ]]; then
    delete_only=1
  else
    nondelete=1
    delete_only=0
    if [[ "$local_ref" == refs/tags/* ]]; then tag_commits+=("$local_oid"); else
      branch_push=1
      [[ "$local_oid" == "$(git rev-parse HEAD)" ]] || { echo 'verify-change: push non-HEAD refs separately from their own checkout' >&2; exit 1; }
    fi
  fi
done
if [[ "$delete_only" == 1 && "$nondelete" == 0 ]]; then echo 'pre-push: only ref deletions'; exit 0; fi
for commit in "${tag_commits[@]}"; do bash scripts/require-main-validation.sh "$commit"; done
if [[ ${#tag_commits[@]} -gt 0 && "$branch_push" == 0 ]]; then exit 0; fi
expected_go=$(awk '$1 == "go" && NF == 2 { print "go" $2; found++ } END { if (found != 1) exit 1 }' go.mod)
[[ $(go env GOVERSION) == "$expected_go" ]] || { echo 'verify-change: select the go.mod toolchain' >&2; exit 1; }
base=$(git merge-base HEAD origin/master)
plan_file=$(mktemp)
trap 'rm -f "$plan_file" "$plan_file.packages"' EXIT
go run scripts/ci-plan.go -base "$base" -worktree >"$plan_file"
command -v jq >/dev/null || { echo 'verify-change: jq is required' >&2; exit 1; }
jq . "$plan_file"
if [[ $(jq -r .full "$plan_file") == true ]]; then
  exec bash .githooks/pre-push-project --full </dev/null
fi
git diff --check "$base"
git diff --cached --check
go test ./docs -count=1
if [[ $(jq -r .frontend "$plan_file") == true ]]; then
  web_files=$(jq -c .webFiles "$plan_file")
  (cd web && bun run check:static && bun run routes:verify && node --test scripts/__tests__/*.test.mjs && SELECTED_WEB_FILES="$web_files" node scripts/test-related.mjs && bun run build:web)
elif [[ $(jq -r .backend "$plan_file") == true ]]; then
  (cd web && bun run build:web)
fi
if [[ $(jq -r .backend "$plan_file") == true ]]; then
  jq -er '.packages[]' "$plan_file" >"$plan_file.packages"
  mapfile -t packages <"$plan_file.packages"
  rm -f "$plan_file.packages"
  [[ ${#packages[@]} -gt 0 ]] || { echo 'empty affected Go package set' >&2; exit 1; }
  format_drift=$(git ls-files -z --cached --others --exclude-standard -- '*.go' | xargs -0 "$(go env GOROOT)/bin/gofmt" -l)
  [[ -z "$format_drift" ]] || { printf '%s\n' "$format_drift" >&2; exit 1; }
  go build ./cmd/server ./cmd/migrate
  go vet "${packages[@]}"
  package_parallelism=2
  if [[ -n ${PG_TEST_DSN:-} ]]; then package_parallelism=1; fi
  go test "${packages[@]}" -race -count=1 -p "$package_parallelism" -timeout=900s
  if [[ -n ${PG_TEST_DSN:-} ]]; then
    PG_TEST_DSN="$PG_TEST_DSN" go test "${packages[@]}" -count=1 -tags=integration -p 1 -timeout=900s
  else
    echo 'PostgreSQL affected behavior NOT verified locally; PR test-pg must verify it.'
  fi
fi
echo 'pre-push: affected change checks passed (not a release approval)'
