#!/usr/bin/env bash
# Cheap, fail-closed checks before starting the embedded build and test lanes.
set -euo pipefail
export GOFLAGS=" "

expected_go=$(awk '$1 == "go" && NF == 2 { print "go" $2; found++ } END { if (found != 1) exit 1 }' go.mod)
actual_go=$(go env GOVERSION)
if [ "$actual_go" != "$expected_go" ]; then
	echo "preflight: Go toolchain mismatch: go.mod requires $expected_go, got $actual_go. Select the pinned toolchain before running the gate." >&2
	exit 1
fi
# gofmt on PATH can belong to a different Go installation. Use the formatter
# shipped with the selected compiler instead.
go_root=$(go env GOROOT)
if [ -z "$go_root" ] || [ ! -x "$go_root/bin/gofmt" ]; then
	echo "preflight: cannot locate gofmt in the selected GOROOT" >&2
	exit 1
fi
git diff --check
git diff --cached --check

file_list=$(mktemp "${TMPDIR:-/tmp}/metapi-gofmt.XXXXXX")
trap 'rm -f "$file_list"' EXIT
# Do not hide a git failure behind process substitution. NUL delimiters keep
# filenames with spaces intact; deleted tracked files fail closed in gofmt.
git ls-files -z -- '*.go' >"$file_list"
mapfile -d '' -t go_files <"$file_list"
if [ "${#go_files[@]}" -eq 0 ]; then
	echo "preflight: no tracked Go files; refusing an empty format scan" >&2
	exit 1
fi
format_drift=$("$go_root/bin/gofmt" -l "${go_files[@]}")
if [ -n "$format_drift" ]; then
	echo "preflight: gofmt drift in these files:" >&2
	printf '%s\n' "$format_drift" >&2
	exit 1
fi
echo "preflight: Go toolchain, diff and ${#go_files[@]} tracked Go files clean"
(cd web && bun run check:static)
