# Release artwork

The art the installers use. Everything here is generated from the app's own source artwork, so the
installer looks like the product and not like a template.

| File | Used by | Size | Source |
|------|---------|------|--------|
| `icon.png` | the Linux icon, and the source for the two below | 1024x1024 | the app icon artwork |
| `icon.icns` | the macOS app icon (Finder, Dock, DMG) | 1024 + 7 sizes | `icon.png` |
| `icon.ico` | the Windows app icon (Explorer, task bar, Start menu) | 16 to 256 | `icon.png` |
| `badgeIcon.icns` | the badge on the mounted DMG volume | 1024 + 7 sizes | `icon.png` |
| `background.tiff` | the DMG window background | 540x380 | the wordmark and the mascot artwork |
| `background@2x.tiff` | the same, on a Retina display | 1080x760 | as above |
| `installerSidebar.bmp` | the NSIS welcome and finish pages | 164x314 | the mascot and the wordmark |

The DMG background keeps the two icon spots clear. Tauri's DMG bundler draws the app icon and the
Applications folder at the centres (130, 215) and (410, 215), so the art stays above them and the
lower half is left open.

## Regenerating

```sh
python3 scripts/make-release-art.py
```

It needs [Pillow](https://python-pillow.org/) (`python3 -m pip install pillow`) and, for the macOS
icon files, `iconutil` from the macOS command line tools. The script writes into `build/` and takes
its artwork from `app/renderer/views/graphics/`, so new mascot or wordmark art lands in the
installers too.
