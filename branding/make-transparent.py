#!/usr/bin/env python3
"""Cuts the C out of branding/logo-c-original.png onto a transparent background.

The background is flat navy and the C has a bright rim all round, so the
navy reachable from the corners (which includes the inside of the C, through
its opening) is the background. Pixels near it get a soft alpha from their
distance to the navy, and the navy is subtracted back out of their colour so
edges don't keep a dark fringe on light backgrounds.
Run from the repo root: python3 branding/make-transparent.py
"""
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "branding" / "logo-c-original.png"
OUT = ROOT / "branding" / "logo-c-transparent.png"

SOLID_BG = 28  # colour distance under which a pixel is plain background
FULL_FG = 90  # and over which it is fully the logo


def main():
    im = Image.open(SRC).convert("RGB")
    a = np.asarray(im).astype(np.float32)
    bg = a[4, 4].copy()
    dist = np.sqrt(((a - bg) ** 2).sum(axis=2))

    # Background = navy-ish pixels connected to the corners.
    seed = Image.fromarray(((dist < 60) * 255).astype(np.uint8)).copy()  # floodfill is a no-op on array-backed images
    h, w = dist.shape
    for xy in [(0, 0), (w - 1, 0), (0, h - 1), (w - 1, h - 1)]:
        ImageDraw.floodfill(seed, xy, 128)
    region = np.asarray(seed) == 128
    # Include the anti-aliased edge just inside the rim.
    band = np.asarray(Image.fromarray((region * 255).astype(np.uint8)).filter(ImageFilter.MaxFilter(7))) > 0

    alpha = np.ones_like(dist)
    soft = np.clip((dist - SOLID_BG) / (FULL_FG - SOLID_BG), 0, 1)
    alpha[band] = soft[band]

    # Un-mix the navy: observed = a*fg + (1-a)*bg  =>  fg = (observed - (1-a)*bg) / a
    safe = np.maximum(alpha, 1e-3)[..., None]
    fg = np.clip((a - (1 - alpha[..., None]) * bg) / safe, 0, 255)
    rgba = np.dstack([fg, alpha * 255]).astype(np.uint8)
    out = Image.fromarray(rgba, "RGBA")

    # Trim to the C with a small even margin, square.
    l, t, r, b = out.getchannel("A").point(lambda v: 255 if v > 8 else 0).getbbox()
    side = max(r - l, b - t) + 40
    sq = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    sq.paste(out.crop((l, t, r, b)), ((side - (r - l)) // 2, (side - (b - t)) // 2))
    sq.save(OUT, optimize=True)
    print(f"background {bg.astype(int).tolist()}, C {l, t, r, b}, wrote {OUT.name} {sq.size}")


if __name__ == "__main__":
    main()
