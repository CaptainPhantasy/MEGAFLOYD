# Rebrand: Kimi 3.2.17 → MEGAFLOYD Desktop

The base bundle is pinned at `vendor/Kimi-3.2.17-arm64.app` and never edited. Every build
copies it and applies the mutations below (`desktop/scripts/build.mjs`).

## Bundle

| Field | Kimi | MEGAFLOYD |
|---|---|---|
| Bundle name / display name | Kimi | MEGAFLOYD |
| Bundle identifier | `com.moonshot.kimichat` | `ai.legacy.megafloyd` |
| Executable | `Contents/MacOS/Kimi` | `Contents/MacOS/MEGAFLOYD` |
| Helper apps | `Kimi Helper*.app` | `MEGAFLOYD Helper*.app` |
| Helper identifiers | `com.moonshot.kimichat.helper` | `ai.legacy.megafloyd.helper*` |
| Version | 3.2.17 (3.2.17.4) | 1.0.0 (1) |
| URL scheme | `kimi`, `kimi-work` | `megafloyd` |
| Icon | Kimi mark | generated `M` mark (`desktop/scripts/make-icon.py`) |
| Copyright | Beijing Moonshot | Legacy AI |
| Category | (none) | `public.app-category.developer-tools` |
| Document types | PDF/Office/ZIP/plain text | removed |
| Localizations | 60+ `.lproj` | removed; `en` only |

## Deletions

| Path | Reason |
|---|---|
| `Contents/Resources/app.asar` | replaced with `desktop/src` |
| `Contents/Resources/app.asar.unpacked` | Kimi native modules (OCR, screen capture, global input) |
| `Contents/Resources/app-update.yml` | pointed at `kimi-img.moonshot.cn` |
| `Contents/Resources/resources/` | Kimi gateway (`openclaw`), daimon bundle, skills, edit3d, webbridge, Kimi binaries |
| `Contents/Resources/*.lproj` | localized Kimi strings would override the product name in Finder |
| `Contents/embedded.provisionprofile` | Moonshot provisioning profile |
| `Contents/_CodeSignature` | stale signature over the old layout |
| `ElectronAsarIntegrity` (Info.plist) | hash cannot survive a replaced asar |

## Fuse change

`EnableEmbeddedAsarIntegrityValidation` is switched **off** with `@electron/fuses` before
signing. The base app ships the fuse on, and the integrity hash is bound to the original
asar header; a forked asar cannot satisfy it. The remaining fuses are untouched:
`OnlyLoadAppFromAsar` stays on, so the app still refuses to run unpacked code.

## Signature

`codesign --force --deep --sign -` (ad-hoc). This is sufficient to run locally; it is not
sufficient for distribution. See the status section of the root README.

## What is kept

The Electron 43.7.3 framework, Chromium helpers, crashpad handler, and the native window
machinery. Those are the parts worth forking; Kimi's product layer is gone.
