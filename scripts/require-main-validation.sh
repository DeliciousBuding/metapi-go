#!/usr/bin/env bash
# Releases may only use an exact SHA with successful main-branch full validation.
set -euo pipefail
sha=$(git rev-parse "${1:?commit required}^{commit}")
repo=${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}
data=$(gh api --method GET "repos/$repo/actions/workflows/main.yml/runs" \
  -f branch=master -f event=push -f head_sha="$sha" -f status=completed -f per_page=100)
# A failed image upload is not a failed source validation. Require the real
# full-suite aggregate, not the conclusion of the subsequent publication chain.
ids=$(printf '%s' "$data" | jq -er --arg sha "$sha" \
  '.workflow_runs[] | select(.head_sha == $sha and .head_branch == "master" and .event == "push") | .id')
for id in $ids; do
  [[ "$id" =~ ^[0-9]+$ ]] || { echo 'invalid Actions run ID' >&2; exit 1; }
  jobs=$(gh api --method GET "repos/$repo/actions/runs/$id/jobs" -f per_page=100)
  if printf '%s' "$jobs" | jq -e '[.jobs[] | select(.name == "docker-build")] | length == 1 and all(.[]; .conclusion == "success")' >/dev/null; then
    echo "master full validation confirmed for $sha (run $id)"
    exit 0
  fi
done
echo "No successful master full validation for exact commit $sha; release refused." >&2
exit 1
