
#!/usr/bin/env python3
"""Builds the release artwork (DMG background, NSIS sidebar, macOS and Windows icons) from the
app's own source art. Run with Pillow installed:
    python3 scripts/make-release-art.py
Sources:
    build/icon.png                       the app icon (source artwork)
    app/renderer/views/graphics/logo.png   the wordmark
    app/renderer/views/graphics/mascots/03-fistbump.png   the mascot
"""
import os, subprocess, sys
from PIL import Image, ImageDraw, ImageFilter

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
B = lambda *p: os.path.join(ROOT, *p)

NAVY_TOP = (16, 44, 99)
NAVY_BOTTOM = (6, 20, 48)
YELLOW = (245, 195, 59)
W = lambda px: (px, px, px, 255)


def gradient(size, top=NAVY_TOP, bottom=NAVY_BOTTOM):
    w, h = size
    img = Image.new("RGBA", size)
    d = ImageDraw.Draw(img)
    for y in range(h):
        t = y / max(1, h - 1)
        d.line([(0, y), (w, y)], fill=tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3)) + (255,))
    return img


def paste_scaled(base, art, box):
    """Scales art to fit inside box (x, y, w, h) and pastes it centered, keeping the ratio."""
    x, y, w, h = box
    art = art.copy()
    art.thumbnail((w, h), Image.LANCZOS)
    ox = x + (w - art.width) // 2
    oy = y + (h - art.height) // 2
    base.alpha_composite(art, (ox, oy))


def glow(base, cx, cy, radius, strength=0.10):
    """A soft light spot, so the icons sit on a lit patch instead of flat colour."""
    w, h = base.size
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    d.ellipse([cx - radius, cy - radius, cx + radius, cy + radius], fill=(255, 255, 255, int(255 * strength)))
    layer = layer.filter(ImageFilter.GaussianBlur(radius / 2))
    base.alpha_composite(layer)


def make_background(path, scale):
    """540x380 at 1x, with the icon spots at (130, 220) and (410, 220) kept clear."""
    W1, H1 = 540, 380
    img = gradient((W1 * scale, H1 * scale))
    word = Image.open(B("app/renderer/views/graphics/logo.png")).convert("RGBA")
    paste_scaled(img, word, (140 * scale, 26 * scale, 260 * scale, 50 * scale))
    d = ImageDraw.Draw(img)
    d.rounded_rectangle([250 * scale, 92 * scale, 290 * scale, 96 * scale], radius=2 * scale, fill=YELLOW + (255,))
    glow(img, 130 * scale, 220 * scale, 120 * scale, 0.10)
    glow(img, 410 * scale, 220 * scale, 120 * scale, 0.10)
    img.convert("RGB").save(path, "TIFF", dpi=(72 * scale, 72 * scale))


def make_sidebar(path):
    """164x314 for the NSIS welcome and finish pages."""
    W1, H1 = 164, 314
    img = gradient((W1, H1))
    mascot = Image.open(B("app/renderer/views/graphics/mascots/03-fistbump.png")).convert("RGBA")
    paste_scaled(img, mascot, (12, 26, 140, 150))
    word = Image.open(B("app/renderer/views/graphics/logo.png")).convert("RGBA")
    paste_scaled(img, word, (22, 196, 120, 26))
    d = ImageDraw.Draw(img)
    d.rounded_rectangle([52, 244, 112, 248], radius=2, fill=YELLOW + (255,))
    img.convert("RGB").save(path, "BMP")


def make_icons():
    """The macOS icon set and the Windows icon, both from build/icon.png."""
    src = Image.open(B("build/icon.png")).convert("RGBA")
    sizes = [16, 32, 64, 128, 256, 512, 1024]
    iconset = B("build/icon.iconset")
    os.makedirs(iconset, exist_ok=True)
    for s in sizes:
        src.resize((s, s), Image.LANCZOS).save(os.path.join(iconset, "icon_%dx%d.png" % (s, s)))
        if s < 512:
            src.resize((s * 2, s * 2), Image.LANCZOS).save(os.path.join(iconset, "icon_%dx%d@2x.png" % (s, s)))
    subprocess.run(["iconutil", "-c", "icns", iconset, "-o", B("build/icon.icns")], check=True)
    src.save(B("build/icon.ico"), sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
    subprocess.run(["iconutil", "-c", "icns", iconset, "-o", B("build/badgeIcon.icns")], check=True)


if __name__ == "__main__":
    make_background(B("build/background.tiff"), 1)
    make_background(B("build/background@2x.tiff"), 2)
    make_sidebar(B("build/installerSidebar.bmp"))
    make_icons()
    for f in ["background.tiff", "background@2x.tiff", "installerSidebar.bmp", "icon.icns", "icon.ico", "badgeIcon.icns"]:
        print(f, os.path.getsize(B("build", f)), "bytes")

