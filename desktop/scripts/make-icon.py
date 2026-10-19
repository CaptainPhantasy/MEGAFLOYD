#!/usr/bin/env python3
"""Generate MEGAFLOYD.app's icon.

Produces desktop/assets/icon.icns from a drawn 1024×1024 master. No external
assets: the mark is geometric so the build stays reproducible.
"""

from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parent.parent
ASSETS = ROOT / "assets"
ICONSET = ASSETS / "MEGAFLOYD.iconset"
MASTER = ASSETS / "icon-1024.png"
OUT = ASSETS / "icon.icns"

BG_TOP = (22, 23, 28, 255)
BG_BOTTOM = (8, 8, 10, 255)
ACCENT = (183, 245, 74, 255)
DIM = (242, 242, 245, 255)


def rounded_mask(size: int, radius: int) -> Image.Image:
    mask = Image.new("L", (size, size), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, size - 1, size - 1), radius=radius, fill=255)
    return mask


def draw_master(size: int = 1024) -> Image.Image:
    # Vertical gradient plate.
    plate = Image.new("RGBA", (size, size))
    pixels = plate.load()
    for y in range(size):
        t = y / (size - 1)
        color = tuple(int(BG_TOP[i] * (1 - t) + BG_BOTTOM[i] * t) for i in range(3)) + (255,)
        for x in range(size):
            pixels[x, y] = color

    draw = ImageDraw.Draw(plate)
    # Hairline inner border catches light on dark desktops.
    draw.rounded_rectangle((3, 3, size - 4, size - 4), radius=226, outline=(255, 255, 255, 22), width=3)

    font_path = "/System/Library/Fonts/Supplemental/Arial Bold.ttf"
    if not Path(font_path).exists():
        font_path = "/System/Library/Fonts/Helvetica.ttc"
    try:
        font = ImageFont.truetype(font_path, int(size * 0.52))
    except OSError:
        font = ImageFont.load_default()

    text = "M"
    bbox = draw.textbbox((0, 0), text, font=font)
    tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
    # Optical centre: textbbox already includes bearing; nudge up slightly.
    draw.text(
        ((size - tw) / 2 - bbox[0], (size - th) / 2 - bbox[1] - size * 0.015),
        text,
        font=font,
        fill=ACCENT,
    )

    # Underline: a short rule that reads at 16 px.
    rule_w, rule_h = int(size * 0.34), int(size * 0.028)
    x0 = (size - rule_w) // 2
    y0 = int(size * 0.70)
    draw.rounded_rectangle((x0, y0, x0 + rule_w, y0 + rule_h), radius=rule_h // 2, fill=DIM)

    plate.putalpha(rounded_mask(size, 226))
    return plate


def main() -> int:
    ASSETS.mkdir(parents=True, exist_ok=True)
    master = draw_master(1024)
    master.save(MASTER)

    if ICONSET.exists():
        shutil.rmtree(ICONSET)
    ICONSET.mkdir(parents=True)

    sizes = [16, 32, 64, 128, 256, 512]
    for base in sizes:
        img = master.resize((base, base), Image.LANCZOS)
        img.save(ICONSET / f"icon_{base}x{base}.png")
        img2 = master.resize((base * 2, base * 2), Image.LANCZOS)
        img2.save(ICONSET / f"icon_{base}x{base}@2x.png")

    subprocess.run(["iconutil", "-c", "icns", str(ICONSET), "-o", str(OUT)], check=True)
    print(f"wrote {OUT} ({OUT.stat().st_size} bytes)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
