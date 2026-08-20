#!/usr/bin/env bash
# next-version.sh — read-only helper for the patch-first release cadence
# (docs/internal/git-workflow.md §6.1).
#
# Prints the next SemVer candidates derived from the latest vX.Y.Z tag. It
# never tags or mutates anything; it only suggests. Pick the patch candidate
# by default; reserve minor for themed milestones and major for the 1.0
# readiness criteria.
#
# Usage:
#   bash scripts/next-version.sh
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

latest="$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -1 || true)"
if [[ -z "$latest" ]]; then
  echo "no version tags found; start at 0.1.0"
  exit 0
fi
ver="${latest#v}"
IFS='.' read -r major minor patch <<< "$ver"
echo "latest tag : $latest"
echo "next patch : $major.$minor.$((patch + 1))   # default cadence (§6.1)"
echo "next minor : $major.$((minor + 1)).0     # themed milestone only"
echo "next major : $((major + 1)).0.0        # 1.0 readiness criteria only"
