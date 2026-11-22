#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd -- "$script_dir/../.." && pwd)"
cd "$project_root"

export PYTHONPATH="dev/python:dev/python/algorithms:dev/python/tests${PYTHONPATH:+:$PYTHONPATH}"
exec conda run -n base --no-capture-output python -m unittest discover -s dev/python/tests -v
