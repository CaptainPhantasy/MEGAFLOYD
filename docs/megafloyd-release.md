# MegaFloyd release packages

MEGAFLOYD installs one complete release: the Go engine, web server and browser assets, and the macOS desktop shell. The terminal launcher uses the Go engine inside that same desktop app. Releases are served from `https://inference.tail58d565.ts.net:8444/megafloyd`; the existing `/code` feed belongs to Floyd7.

Build the host Go executable with `scripts/build-megafloyd.sh --host --version VERSION --build-id BUILD_ID --out OUTPUT`. The build applies release symbol stripping and produces `SHA256SUMS` plus `build-receipt.json`. Use the same three-part product version and unique build ID for every component.

Stage the production web package with the repository's pinned pnpm command, preserving the existing browser bundle:

```sh
pnpm --filter @legacy-ai/floyd-code deploy --prod --legacy --offline ABSOLUTE_WEB_STAGE
```

Run `desktop/desktop/scripts/build.mjs` with `MEGAFLOYD_VERSION`, `MEGAFLOYD_BUILD_ID`, `MEGAFLOYD_SOURCE_COMMIT`, `MEGAFLOYD_PLATFORM_BIN`, `MEGAFLOYD_PLATFORM_WEB`, `MEGAFLOYD_DESKTOP_OUT`, and `MEGAFLOYD_DESKTOP_BUILD_DIR`. `MEGAFLOYD_BASE` names the preserved Electron base or existing canonical MEGAFLOYD app. The builder creates a fresh staging directory, stamps desktop and web release metadata, verifies the signed output, and backs up any existing output before replacement. Web protocol version2 remains independent of the product's release version.

Package the finished application:

```sh
python3 scripts/megafloyd-release.py package \
  --repo ABSOLUTE_CANONICAL_CHECKOUT \
  --app ABSOLUTE_STAGED_MEGAFLOYD_APP \
  --out ABSOLUTE_NEW_RELEASE_DIRECTORY \
  --version VERSION --build-id BUILD_ID \
  --build-receipt ABSOLUTE_GO_BUILD_RECEIPT --go ABSOLUTE_VERIFIED_GO
```

The output contains the full Mac archive, `manifest.json`, `latest.json`, `install.sh`, and `SHA256SUMS`. The manifest records the component checksums, source-input digest, platform, and common release identity. Packaging rejects mismatched identities, broken links, and links that escape the bundled release. Keep the release directory immutable after publication. Publish versioned assets and their manifest first; publish the new `latest.json` last.

Install from local artifacts with an explicit destination:

```sh
python3 scripts/megafloyd-release.py install \
  --prefix ABSOLUTE_INSTALL_PREFIX \
  --manifest ABSOLUTE_RELEASE_DIRECTORY/manifest.json
```

The downloaded `install.sh` accepts the same `--prefix`, `--manifest`, `--archive`, `--check`, and `--verify-only` options. Without a local manifest it uses MEGAFLOYD's own CDN namespace. No destination is selected automatically.

Installation verifies the entire archive and file inventory, installs `<prefix>/releases/<release_id>`, and switches `<prefix>/current` with an atomic rename. `<prefix>/bin/megafloyd` and `<prefix>/MEGAFLOYD.app` both select that release. Existing entry points and receipts receive dated backups. A failed activation check restores the previous links and receipt. The installed terminal command provides `megafloyd update --check` and `megafloyd update` through the same package installer.

An update changes code and assets only. Existing processes retain their immutable release until the identified idle MEGAFLOYD runtime is restarted. Preserve the workspace/session when coordinating that restart. Personal state defaults to the current account's `~/.megafloyd`, with private workspace stores under `data/workspaces/<canonical-workspace-hash>`. An explicit `--data-dir` or this account's `state/workspace-data.json` mapping can preserve a deliberately selected existing store. Provider credentials, model settings, session databases, memories, and tasks are never included in the package or overwritten by the installer. Database recovery is a separate coordinated operation if a future release changes compatibility.

The focused package check uses the actual built archive and no model calls:

```sh
python3 scripts/verify-megafloyd-package.py \
  --installer ABSOLUTE_CANONICAL_CHECKOUT/scripts/megafloyd-release.py \
  --manifest ABSOLUTE_RELEASE_DIRECTORY/manifest.json \
  --workspace ABSOLUTE_CANONICAL_VERIFICATION_DIRECTORY
```

It proves successful complete installation, rejection of a corrupted archive, restoration after a controlled post-switch executable failure, preservation of unrelated files, and routing of the installed update command. The receipt records the exact archive hash tested.

Current release: [installed completion](megafloyd-completion.md) and [234343 release verification](megafloyd-release-verification.md). See [the current environment and state layout](megafloyd-namespace.md), [login startup and telemetry](megafloyd-startup-telemetry-verification.md), [custom frontend preservation](megafloyd-web-branding-preservation.md), and [workstation MCP onboarding](megafloyd-mcp-onboarding.md). The installed executable was built from implementation `8c4876a3d166d9ac1ddd9952f27b2ab0bed3dfd3`; later documentation and ship-manifest metadata commits do not rebuild it.
