#!/usr/bin/env python3
"""Builds the app icons in web/public from branding/logo-c-original.png.

The source is the C on a flat navy square. The C is re-centred and padded
(the navy extends seamlessly), then placed on a rounded tile with
transparent corners. Small sizes get a bigger C and tighter corners so it
still reads in a browser tab. Run from the repo root: python3 branding/make-icons.py
"""
from pathlib import Path

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "branding" / "logo-c-original.png"
OUT = ROOT / "web" / "public"

LARGE = [64, 128, 180, 192, 256, 512, 1024]
SMALL = [16, 32, 48]


def c_bounds(im, bg, tolerance=40):
    """Bounding box of everything that isn't the background."""
    mask = Image.eval(im.convert("L"), lambda v: 0)
    px, w, h = im.load(), *im.size
    m = mask.load()
    for y in range(h):
        for x in range(w):
            if sum(abs(a - b) for a, b in zip(px[x, y], bg)) > tolerance:
                m[x, y] = 255
    return mask.getbbox()


def square(im, bg, box, fill):
    """A square crop with the C centred, taking `fill` of the width."""
    l, t, r, b = box
    side = round(max(r - l, b - t) / fill)
    cx, cy = (l + r) / 2, (t + b) / 2
    canvas = Image.new("RGB", (side, side), bg)
    canvas.paste(im, (round(side / 2 - cx), round(side / 2 - cy)))
    return canvas


def rounded(img, size, radius):
    """img resized to size, with corners of radius (a fraction of size) cut away."""
    out = img.resize((size, size), Image.LANCZOS).convert("RGBA")
    ss = 8  # supersampled mask for smooth corners
    mask = Image.new("L", (size * ss, size * ss), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, size * ss - 1, size * ss - 1), radius=radius * size * ss, fill=255)
    out.putalpha(mask.resize((size, size), Image.LANCZOS))
    return out


def main():
    im = Image.open(SRC).convert("RGB")
    bg = im.getpixel((4, 4))
    box = c_bounds(im, bg)
    large = square(im, bg, box, 0.72)
    small = square(im, bg, box, 0.86)

    icons = OUT / "icons"
    for s in LARGE:
        rounded(large, s, 0.22).save(icons / f"logo-{s}.png", optimize=True)
    for s in SMALL:
        rounded(small, s, 0.18).save(icons / f"logo-{s}.png", optimize=True)

    # iOS masks the icon itself and shows transparency as black: full square.
    large.resize((180, 180), Image.LANCZOS).save(OUT / "apple-touch-icon.png", optimize=True)
    # A maskable icon for installed web apps: full-bleed navy (Android crops
    # it to its own shape), the C at 60% so it stays inside the 80% safe zone.
    square(im, bg, box, 0.60).resize((512, 512), Image.LANCZOS).save(icons / "maskable-512.png", optimize=True)

    ico = [rounded(small, s, 0.18) for s in SMALL]
    ico[-1].save(OUT / "favicon.ico", sizes=[(s, s) for s in SMALL], append_images=ico[:-1])
    print(f"C at {box}, background {bg}; wrote {len(LARGE) + len(SMALL)} PNGs, apple-touch-icon.png and favicon.ico")


if __name__ == "__main__":
    main()
