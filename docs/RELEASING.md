# Releasing goports

## Version tags

1. Ensure `main`/`master` is green (CI: vet, test, build, MVS lint).
2. Optionally run `make mvs-generate` and commit if decorators or CLI surface changed.
3. Create an annotated or lightweight tag: `git tag vX.Y.Z` (must match `v*` for [`.github/workflows/release.yml`](../.github/workflows/release.yml)).
4. `git push origin vX.Y.Z`.

The **Release** workflow builds Linux (amd64/arm64), Windows (amd64), and macOS (amd64/arm64) binaries, writes `checksums.txt`, and publishes a GitHub release with auto-generated notes.

Set the link-time version string with the same `v`-stripped value (the workflow passes `-X ...internal/version.Version=` automatically).

## Release notes habit

Auto-generated GitHub notes list commits; add a short **human summary** when publishing (or edit the release after the workflow):

- What users should care about (new flags, GUI changes, HTTP/API tweaks).
- Any breaking CLI or API behavior.
- Upgrade hints (e.g. reset preferences, new env vars).

Copy from [`RELEASE_NOTES.template.md`](RELEASE_NOTES.template.md) if it helps you stay consistent.

## macOS app bundle / signing (optional)

ZIP releases are unsigned. For Gatekeeper-friendly `.app` distribution:

1. Build `goports.app` (`make build-app`).
2. Sign with a **Developer ID Application** certificate: `codesign --deep --force --options runtime --sign "Developer ID Application: …" goports.app`.
3. Notarize: `xcrun notarytool submit goports.zip --apple-id … --team-id … --password … --wait`.
4. Staple: `xcrun stapler staple goports.app`.

Details vary by Apple account and CI secrets; see [`MACOS_SIGNING.md`](MACOS_SIGNING.md).
