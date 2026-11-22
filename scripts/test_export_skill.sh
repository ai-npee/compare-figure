#!/usr/bin/env bash
# Focused smoke tests for argument validation; no Go build or export is needed.
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script="${script_dir}/export_skill.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/compare-figure-export-test.XXXXXX")
cleanup() { rm -rf -- "$tmp"; }
trap cleanup EXIT INT TERM

"$script" --help >/dev/null
if "$script" --version '1.2' --skip-build --out-dir "$tmp/out" >/dev/null 2>&1; then
    printf 'invalid semver unexpectedly accepted\n' >&2
    exit 1
fi
if "$script" --bogus >/dev/null 2>&1; then
    printf 'unknown option unexpectedly accepted\n' >&2
    exit 1
fi
if "$script" --version '1.2.3/evil' --skip-build --out-dir "$tmp/out" >/dev/null 2>&1; then
    printf 'unsafe version unexpectedly accepted\n' >&2
    exit 1
fi
if "$script" --version '1.2.3-01' --skip-build --out-dir "$tmp/out" >/dev/null 2>&1; then
    printf 'numeric prerelease with leading zero unexpectedly accepted\n' >&2
    exit 1
fi
printf 'export_skill argument tests passed\n'
