#!/usr/bin/env python3
"""
Generate PWA + extension icons from a single in-code recipe. Run via:

    services/nlp/.venv/bin/python scripts/gen-icons.py

Produces:
    apps/web/static/icon-{192,256,384,512}.png
    apps/web/static/apple-touch-icon.png
    apps/web/static/favicon-32.png
    apps/web/static/favicon-16.png

The recipe: a deep slate-charcoal square (#13151a) with a centered carved
"C" mark in the brand green (#4caf50). Rendered procedurally with Pillow
so the same logic produces every size — no SVG → PNG external dependency,
no hand-edited per-size PNGs to drift apart.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

from PIL import Image, ImageDraw

REPO = Path(__file__).resolve().parents[1]
OUT  = REPO / "apps" / "web" / "static"

BG   = (19, 21, 26, 255)      # #13151a — app shell background
FG   = (76, 175, 80, 255)     # #4caf50 — carve brand green

SIZES = [
    ("icon-192.png", 192, True),
    ("icon-256.png", 256, True),
    ("icon-384.png", 384, True),
    ("icon-512.png", 512, True),
    ("apple-touch-icon.png", 180, True),
    ("favicon-32.png", 32, False),
    ("favicon-16.png", 16, False),
]


def render_icon(size: int, rounded: bool) -> Image.Image:
    """
    Render an icon at `size` × `size`. The mark is a stylized "C" — a
    270° arc whose stroke width tracks the canvas size.
    """
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # Background tile (rounded square on larger sizes; flat at favicon sizes
    # where rounding looks fuzzy).
    if rounded:
        radius = max(2, size // 8)
        draw.rounded_rectangle((0, 0, size - 1, size - 1), radius=radius, fill=BG)
    else:
        draw.rectangle((0, 0, size - 1, size - 1), fill=BG)

    # The "C" mark — a 270° arc from -135° to +135° (open on the right).
    margin = size * 0.18
    box = (margin, margin, size - margin, size - margin)
    width = max(2, int(size * 0.14))
    draw.arc(box, start=135, end=45, fill=FG, width=width)

    # A small filled square at the right opening to suggest a "carved"
    # cut, giving the mark its identity.
    cut = size * 0.10
    cx = size - margin - cut * 0.6
    cy = size / 2
    draw.rounded_rectangle(
        (cx - cut / 2, cy - cut / 2, cx + cut / 2, cy + cut / 2),
        radius=max(1, int(cut * 0.25)), fill=FG,
    )
    return img


def main() -> int:
    OUT.mkdir(parents=True, exist_ok=True)
    for name, size, rounded in SIZES:
        path = OUT / name
        img = render_icon(size, rounded)
        img.save(path, format="PNG", optimize=True)
        print(f"  wrote {path.relative_to(REPO)} ({size}×{size})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
