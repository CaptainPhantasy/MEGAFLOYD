# MegaFloyd application icons

The owner-provided arcade artwork is kept unchanged at `assets/branding/megafloyd-arcade.png`. Its source SHA-256 is `d2e6da2d7edc6cc66c4c84eabda76cc2e47924e56b2cd540c88c587aed22a105`.

Run `swift scripts/build-megafloyd-icons.swift` on macOS to produce the desktop ICNS, browser PNG and ICO variants, Go notification images, and canonical documentation/VS Code icons. The former `desktop/desktop/scripts/make-icon.py` entry point runs this same converter. It no longer draws the old M icon. Conversion preserves transparency and aspect ratio, fits the complete artwork in a square with a small transparent margin, and saves replaced assets under a dated `.tmp/icon-backups-*` directory.

The source artwork and consumer assets are committed. Update the preserved web asset manifest only after reviewing intentional generated changes; the normal build checks that manifest and never refreshes it. Browser icon URLs carry the artwork hash to replace cached legacy icons. The macOS Dock, window, menu bar, Finder and desktop notifications use the signed app's `icon.icns`. Go notification consumers embed the same artwork when built. The terminal's textual wordmark and the accepted animated web wordmark/founder heads remain separate artwork.
