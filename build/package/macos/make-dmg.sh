#!/bin/sh
# Packages Goi.app as a drag-to-Applications disk image.
#   build/package/macos/make-dmg.sh build/bin/Goi.app dist/Goi-0.1.0-macos-universal.dmg
set -e
app="${1:?usage: make-dmg.sh <Goi.app> <out.dmg>}"
out="${2:?usage: make-dmg.sh <Goi.app> <out.dmg>}"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
cp -R "$app" "$stage/"
ln -s /Applications "$stage/Applications"
mkdir -p "$(dirname "$out")"
rm -f "$out"
hdiutil create -volname "Goi" -srcfolder "$stage" -fs HFS+ -format UDZO -ov "$out" >/dev/null
echo "$out"
