# macOS signing and notarization

These steps are **not** run in CI by default (they require Apple Developer credentials). Use them when you need users to open `goports.app` without Gatekeeper warnings.

## Prerequisites

- Apple Developer Program membership.
- **Developer ID Application** certificate installed in Keychain (or exported `.p12` for CI).
- App-specific password for notarytool (if using Apple ID auth).

## Local signing

```bash
make build-app
codesign --deep --force --options runtime \
  --sign "Developer ID Application: Your Name (TEAMID)" \
  goports.app
```

Verify:

```bash
codesign -dv --verbose=4 goports.app
```

## Notarization

Zip the app, then submit:

```bash
ditto -c -k --keepParent goports.app goports.zip
xcrun notarytool submit goports.zip \
  --apple-id "you@example.com" \
  --team-id "TEAMID" \
  --password "@keychain:AC_PASSWORD" \
  --wait
xcrun stapler staple goports.app
```

## CI (optional)

To automate later, store secrets such as `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, and a base64-encoded `MACOS_CERT_P12` + `MACOS_CERT_PASSWORD`, then run the same commands in a `macos-latest` job **after** `make build-app`. Keep signing jobs behind manual approval or restricted environments.

## Threat model reminder

Signing proves publisher identity; it does not replace careful review of what the binary does. Keep release artifacts and checksums (`checksums.txt` on GitHub releases) as the source of truth for downloaded CLI binaries.
