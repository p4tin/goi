#!/bin/sh
# Regenerates an app's icons from apps/<app>/build/appicon.svg (macOS: needs
# Google Chrome for SVG rendering and python3):
#   scripts/icons.sh [app]                 app defaults to goi
#   apps/<app>/build/appicon.png           1024 px, macOS icon grid (Wails makes the .icns; Linux packages use it too)
#   apps/<app>/build/windows/icon.ico      16–256 px, full-bleed (no macOS margin); also appicon.ico (file associations)
# Copy the svg to the app's frontend pack icon (frontend/src/packs/<pack>/icon.svg)
# too: Welcome and About show it.
set -e
cd "$(dirname "$0")/.."
app="${1:-goi}"
dir="apps/$app/build"
[ -f "$dir/appicon.svg" ] || { echo "icons: no $dir/appicon.svg" >&2; exit 1; }
chrome=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# render <svg> <size> <out.png>
render() {
  printf '<html><body style="margin:0;background:transparent"><img src="file://%s" width="%s" height="%s"></body></html>' "$1" "$2" "$2" > "$tmp/r.html"
  "$chrome" --headless=new --disable-gpu --hide-scrollbars --default-background-color=00000000 \
    --window-size="$2,$2" --screenshot="$3" "file://$tmp/r.html" >/dev/null 2>&1
}

src="$PWD/$dir/appicon.svg"
render "$src" 1024 "$dir/appicon.png"

# Windows icons fill the square: crop the viewBox to the rounded rectangle.
sed 's/viewBox="0 0 1024 1024"/viewBox="100 100 824 824"/' "$src" > "$tmp/full.svg"
for s in 16 20 24 32 40 48 64 128 256; do
  render "$tmp/full.svg" "$s" "$tmp/ico-$s.png"
done
mkdir -p "$dir/windows"
python3 - "$tmp" "$dir/windows/icon.ico" <<'PY'
import struct, sys
tmp, out = sys.argv[1], sys.argv[2]
sizes = [16, 20, 24, 32, 40, 48, 64, 128, 256]
pngs = [open(f"{tmp}/ico-{s}.png", "rb").read() for s in sizes]
offset = 6 + 16 * len(sizes)
head = struct.pack("<HHH", 0, 1, len(sizes))
entries = b""
for s, data in zip(sizes, pngs):
    entries += struct.pack("<BBBBHHII", s % 256, s % 256, 0, 0, 1, 32, len(data), offset)
    offset += len(data)
open(out, "wb").write(head + entries + b"".join(pngs))
PY
# The NSIS file associations (iconName: appicon in build/config.yml) install windows/appicon.ico.
cp "$dir/windows/icon.ico" "$dir/windows/appicon.ico"
echo "wrote $dir/appicon.png, $dir/windows/icon.ico and $dir/windows/appicon.ico"
