#!/usr/bin/env bash
# Release helper — validates a release is consistent, then tags and pushes.
# The CI/CD pipeline (.github/workflows/main.yml) builds images + release
# binaries + the GitHub Release for SemVer tags only.
#
# Usage:
#   scripts/release.sh 0.11.0            # validate + tag + push
#   scripts/release.sh 0.11.0 --dry-run  # validate only
#
# Preconditions (checked here):
#   - HEAD equals origin/master (any clean checkout/worktree)
#   - CHANGELOG.md contains "## [v0.11.0]" section (release body source)
#   - web/package.json version == 0.11.0 (binary/SPA version sync)
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

ver="${1:-}"
if [[ -z "$ver" ]]; then
  echo "usage: scripts/release.sh <version> [--dry-run]" >&2
  echo "  version: semver without leading 'v', e.g. 0.11.0" >&2
  exit 2
fi
if ! [[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid semver: $ver (expected X.Y.Z)" >&2
  exit 2
fi
tag="v$ver"
dry=0
[[ "${2:-}" == "--dry-run" ]] && dry=1

# --- preconditions ---
git fetch origin master --quiet
local_head="$(git rev-parse HEAD)"
remote_head="$(git rev-parse origin/master)"
if [ "$local_head" != "$remote_head" ]; then
  echo "HEAD is not exactly synchronized with origin/master; use a clean checkout of the release commit" >&2
  echo "  local : $local_head" >&2
  echo "  remote: $remote_head" >&2
  exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "working tree is dirty; commit or stash first" >&2
  exit 1
fi
if git rev-parse --verify "refs/tags/$tag" >/dev/null 2>&1; then
  echo "tag $tag already exists" >&2
  exit 1
fi

# --- consistency checks ---
if ! grep -q "^## \[$tag\]" CHANGELOG.md; then
  echo "CHANGELOG.md is missing section '## [$tag]' (release body source)" >&2
  exit 1
fi
pkg_ver="$(node -p "require('./web/package.json').version")"
if [ "$pkg_ver" != "$ver" ]; then
  echo "web/package.json version ($pkg_ver) does not match $ver" >&2
  exit 1
fi

echo "release checks passed: $tag (CHANGELOG + web/package.json consistent)"
if [ "$dry" = "1" ]; then
  echo "dry-run: tag not created, nothing pushed"
  exit 0
fi

# The project pre-push gate still runs. If it rejects the push (for example,
# missing dependencies in a fresh checkout), do not leave a misleading local tag.
git tag -a "$tag" -m "$tag"
if ! git push origin "refs/tags/$tag:refs/tags/$tag"; then
  if remote_tag="$(git ls-remote --tags origin "refs/tags/$tag")" && [ -z "$remote_tag" ]; then
    git tag -d "$tag" >/dev/null
  fi
  exit 1
fi
echo "pushed $tag — CI/CD will build the image and create the GitHub Release"
