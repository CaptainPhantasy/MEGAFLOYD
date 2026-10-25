#!/usr/bin/env python3
"""Regenerate MegaFloyd app icons from the canonical owner-provided artwork."""

from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[3]

if __name__ == "__main__":
    subprocess.run(["/usr/bin/swift", str(ROOT / "scripts/build-megafloyd-icons.swift")], check=True)
