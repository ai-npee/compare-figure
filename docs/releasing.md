# Releasing

The Skill is distributed as a source-only ZIP plus a SHA-256 sidecar. The archive includes production Go source and required runtime resources; it does not include a prebuilt executable, development tests, Python tooling, or runtime data. Users build it on the target machine with Go 1.25 or newer.

## Checks

GitHub Actions runs on pushes, pull requests, and `v*` tags. The read-only CI jobs run `go test ./...` and `go vet ./...`, test the viewer on Node.js 22 and 24, and exercise the export-script argument checks. The Go job installs TeX, Lua, and Poppler integration tools, exports a source archive, verifies every manifest SHA-256 and the archive checksum, checks that binaries and development/runtime files are absent, then unpacks and builds the exported Go source.

Tests that explicitly detect unavailable host tools may skip locally. CI installs the relevant toolchain so those integration tests can run where supported.

## Publish a version

1. Update the root `VERSION` file and keep the Skill metadata in the same commit aligned with that version.
2. Run the CI checks locally and review the source-only export and its manifest.
3. Create and push an annotated tag whose exact name is `v` followed by the version in `VERSION`, for example `v0.2.2`.

The release workflow only responds to `v*` tags. It rejects a tag that differs from `v$(VERSION)`, exports the tagged source with `scripts/export_skill.sh`, validates the ZIP checksum, and attaches only the ZIP and `.sha256` sidecar to a GitHub Release. The workflow does not publish executables. Prerelease versions (for example `v0.3.0-rc.1`) are published as prereleases and are not marked as the latest stable release. A release is published only after the corresponding CI checks succeed.

Creating/pushing a tag is the release trigger; do not tag or publish without release authorization. The workflows in this repository do not push commits or tags.

The configuration and its export/verification step have been checked locally. They have not yet been pushed or executed on GitHub; the first remote run must confirm the Ubuntu package installation and hosted-runner environment.
