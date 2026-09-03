#!/bin/sh
# Bounded retry around `bun install --frozen-lockfile`.
#
# WHY THIS EXISTS
# bun fetches tarballs from the npm registry through a CDN, and a corrupt or
# truncated download surfaces as "Integrity check failed for tarball: <pkg>" /
# "Fail extracting tarball from <pkg>". That is not a property of this
# repository -- the same commit installs cleanly on a rerun -- but it fails a
# required check and blocks a merge. Observed on 2026-09-03 alone, four
# different packages across three different jobs: recharts (a11y),
# @base-ui/react and @oxlint/binding-linux-x64-gnu (visual-regression), and
# lightningcss-linux-arm64-musl (docker-push, inside the image build).
#
# WHAT IT DELIBERATELY DOES NOT DO
# Retry anything else. A frozen-lockfile mismatch, a missing manifest or a
# lifecycle-script error is a real defect; retrying it three times only delays
# the truth and burns runner minutes. The classifier below is the whole point of
# this script -- a blanket retry would be indistinguishable from hiding
# breakage.
#
# The install cache is cleared before each retry. Without that, a corrupt
# tarball bun already wrote to its cache would be re-read on every attempt and
# the retry would fail identically three times, i.e. not be a retry at all.
#
# Shared by CI (every web-deps step in .github/workflows/main.yml) and the
# image build (Dockerfile stage 1) on purpose: one retry policy, so the two
# cannot drift. scripts/bun_install_wiring_test.go is the gate that keeps them
# pointing here -- a bare `bun install` in a workflow or the Dockerfile turns it
# red. POSIX sh only: the alpine bun image has no bash, so no arrays, no
# PIPESTATUS, no [[ ]].
set -u

MAX_ATTEMPTS="${BUN_INSTALL_MAX_ATTEMPTS:-3}"
TRANSIENT='Integrity check failed|IntegrityCheckFailed|Fail extracting tarball|ECONNRESET|ETIMEDOUT|EAI_AGAIN|ENOTFOUND|socket hang up|fetch failed'

attempt=1
while [ "$attempt" -le "$MAX_ATTEMPTS" ]; do
  log="$(mktemp)"
  bun install --frozen-lockfile >"$log" 2>&1
  rc=$?
  cat "$log"

  if [ "$rc" -eq 0 ]; then
    rm -f "$log"
    if [ "$attempt" -gt 1 ]; then
      echo "bun install succeeded on attempt $attempt of $MAX_ATTEMPTS"
    fi
    exit 0
  fi

  if grep -qE "$TRANSIENT" "$log" && [ "$attempt" -lt "$MAX_ATTEMPTS" ]; then
    echo "::warning::bun install hit a transient registry/CDN fault (attempt $attempt of $MAX_ATTEMPTS); clearing the install cache and retrying"
    grep -E 'Integrity check failed|Fail extracting tarball|^error:' "$log" | head -5 | sed 's/^/    /'
    rm -f "$log"
    bun pm cache rm >/dev/null 2>&1 || true
    sleep $((attempt * 5))
    attempt=$((attempt + 1))
    continue
  fi

  if [ "$attempt" -ge "$MAX_ATTEMPTS" ]; then
    echo "::error::bun install still failing after $attempt attempts"
  else
    echo "::error::bun install failed with a non-transient error; not retrying"
  fi
  rm -f "$log"
  exit "$rc"
done
