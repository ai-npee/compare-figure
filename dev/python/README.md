# Python Development Prototype

Python sources here are development-only prototypes and comparison tests. The production entry point is the Go implementation under the project-root `go/` directory. This directory is not packaged as a production bridge.

## Relocated Files

- Root Python modules: `compare.py`, `compare_backend.py`, `luadraw_docs.py`, `luadraw_harness.py`, `luadraw_runtime.py`, `runner.py`, `tools_cli.py`, `vector_measure.py`, `vector_metrics.py`, `vector_session.py`, and `vector_tools.py`.
- Algorithms: `algorithms/`.
- Tests: `tests/`.
- Python-only support files: `requirements.txt` and the historical `viewer.html`.
- The Python prototype config remains at project root as `examples/runner.json`.

## Tests

Use the project conda `base` environment. From the project root, run:

```sh
dev/python/run_tests.sh
```

Equivalent direct command:

```sh
PYTHONPATH=dev/python:dev/python/algorithms:dev/python/tests python -m unittest discover -s dev/python/tests -v
```

Python modules resolve project resources from the repository root: `prompts/`, `vendor/`, `scripts/`, `.deps/`, `build/`, `go/`, and `runs/` remain in their existing root locations. Test fixtures use isolated temporary directories.

Install prototype dependencies with `python -m pip install -r dev/python/requirements.txt` in the development environment. The former small-image translation failure in `compare.py` is fixed: pyramid levels never exceed their canonical canvas, avoiding zero-area grid cells and invalid scores. Regression tests verify the original exact translation, pixel-perfect alignment, grid boundaries and distinct/deduplicated starting anchors. This development-only fix does not modify the production Go scorer or its receipt.
