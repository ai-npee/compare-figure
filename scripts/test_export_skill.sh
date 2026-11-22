#!/usr/bin/env bash
# Focused smoke tests for source-export argument validation.
set -euo pipefail
script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script="${script_dir}/export_skill.sh"
"$script" --help >/dev/null
if "$script" --version '1.2' >/dev/null 2>&1; then
    printf 'invalid semver unexpectedly accepted\n' >&2
    exit 1
fi
if "$script" --bogus >/dev/null 2>&1; then
    printf 'unknown option unexpectedly accepted\n' >&2
    exit 1
fi
if "$script" --platforms linux/amd64 >/dev/null 2>&1; then
    printf 'binary platform option unexpectedly accepted\n' >&2
    exit 1
fi
if "$script" --skip-build >/dev/null 2>&1; then
    printf 'binary reuse option unexpectedly accepted\n' >&2
    exit 1
fi
if "$script" --version '1.2.3/evil' >/dev/null 2>&1; then
    printf 'unsafe version unexpectedly accepted\n' >&2
    exit 1
fi
if "$script" --version '1.2.3-01' >/dev/null 2>&1; then
    printf 'numeric prerelease with leading zero unexpectedly accepted\n' >&2
    exit 1
fi
printf 'export_skill argument tests passed\n'
