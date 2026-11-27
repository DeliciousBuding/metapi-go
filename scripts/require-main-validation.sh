#!/usr/bin/env bash
# Prove the tested source, independently of later edge publication.
set -euo pipefail
sha=$(git rev-parse "${1:?commit required}^{commit}")
repo=${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}
for workflow in main nightly; do
  filters=(-f branch=master -f status=completed -f per_page=100)
  if [[ "$workflow" == main ]]; then filters+=(-f event=push -f head_sha="$sha"); fi
  runs=$(gh api --method GET "repos/$repo/actions/workflows/$workflow.yml/runs" --paginate --slurp "${filters[@]}")
  ids=$(jq -r --arg sha "$sha" --arg workflow "$workflow" '.[] | .workflow_runs[] |
    select(.head_branch == "master" and .status == "completed") |
    select(if $workflow == "main" then .event == "push" and .head_sha == $sha
      else (.event == "schedule" or .event == "workflow_dispatch") and .conclusion == "success" and
        (.head_sha == $sha or (.event == "workflow_dispatch" and .display_title == ("Full integration " + $sha))) end) | .id' <<< "$runs")
  for id in $ids; do
    [[ "$id" =~ ^[0-9]+$ ]]
    jobs=$(gh api --method GET "repos/$repo/actions/runs/$id/jobs" --paginate --slurp -f per_page=100)
    # A dispatcher may test an older exact SHA. The run title is a lookup hint;
    # only the successful gate's resolved-SHA step identifies the tested source.
    valid=$(jq --arg sha "$sha" --arg workflow "$workflow" '
      [ .[] | .jobs[] ] as $jobs |
      def passed($name): [$jobs[] | select(.name == $name)] | length == 1 and .[0].conclusion == "success";
      def step($job; $name): [$jobs[] | select(.name == $job) | .steps[] | select(.name == $name)] |
        length == 1 and .[0].conclusion == "success";
      if $workflow == "nightly" then passed("nightly-gate") and step("nightly-gate"; "Validated " + $sha)
      else passed("docker-build") and passed("a11y") and passed("test-pg") and passed("test-sqlite") and
        passed("test-e2e") and step("docker-build"; "Build Docker image") and step("docker-build"; "Smoke Docker image") end' <<< "$jobs")
    if [[ "$valid" == true ]]; then
      echo "$workflow full validation confirmed for $sha (run $id)"
      exit 0
    fi
  done
done
echo "No successful full validation for exact commit $sha; release refused." >&2
exit 1
