#!/bin/sh
# Regenerates the app icons from build/appicon.svg (macOS: needs Google Chrome
# for SVG rendering, sips and python3):
#   build/appicon.png       1024 px, macOS icon grid (Wails makes the .icns; Linux packages use it too)
#   build/windows/icon.ico  16–256 px, full-bleed (no macOS margin)
set -e
cd "$(dirname "$0")/.."
chrome=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# render <svg> <size> <out.png>
render() {
  printf '<html><body style="margin:0;background:transparent"><img src="file://%s" width="%s" height="%s"></body></html>' "$1" "$2" "$2" > "$tmp/r.html"
  "$chrome" --headless=new --disable-gpu --hide-scrollbars --default-background-color=00000000 \
    --window-size="$2,$2" --screenshot="$3" "file://$tmp/r.html" >/dev/null 2>&1
}

src="$PWD/build/appicon.svg"
render "$src" 1024 build/appicon.png

# Windows icons fill the square: crop the viewBox to the rounded rectangle.
sed 's/viewBox="0 0 1024 1024"/viewBox="100 100 824 824"/' "$src" > "$tmp/full.svg"
for s in 16 20 24 32 40 48 64 128 256; do
  render "$tmp/full.svg" "$s" "$tmp/ico-$s.png"
done
python3 - "$tmp" build/windows/icon.ico <<'PY'
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
echo "wrote build/appicon.png and build/windows/icon.ico"
