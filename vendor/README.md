# Vendored references

- `luadraw-3.7/luadraw/` is the unmodified LuaDraw 3.7 distribution by Patrick Fradin, copied from the already pinned YanTrace dependency. Source: https://github.com/pfradin/luadraw / https://ctan.org/pkg/luadraw. License: LPPL 1.3c or later, stated in the upstream README and runtime headers. `dependencies.lock.json` records all upstream file SHA-256 values. **0.3.6 is the adjacent exam-zh version, not this LuaDraw version.**
- `docvision-skills/` contains reference index/generator/two-stage Lua expansion scripts and MIT license from LingNc/DocVision's locally available `origin/feat/skills` revision `f0c00e9`. They are reference artifacts; the active independently callable implementation is in the project Python modules. No source branch was switched or edited.

Generated document caches, rendered crops, compiled PDFs and local binary dependencies are not part of this directory; they belong in ignored build/runs directories. Runtime/source/manual files here are pinned inputs, not session outputs.
