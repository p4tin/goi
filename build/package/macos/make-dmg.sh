#!/bin/sh
# Packages an app bundle as a drag-to-Applications disk image.
#   build/package/macos/make-dmg.sh bin/goi/Goi.app dist/Goi-0.1.0-macos-universal.dmg [volume name]
# The volume name defaults to the bundle name without .app.
set -e
app="${1:?usage: make-dmg.sh <App.app> <out.dmg> [volume name]}"
out="${2:?usage: make-dmg.sh <App.app> <out.dmg> [volume name]}"
vol="${3:-$(basename "$app" .app)}"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
cp -R "$app" "$stage/"
ln -s /Applications "$stage/Applications"
mkdir -p "$(dirname "$out")"
rm -f "$out"
hdiutil create -volname "$vol" -srcfolder "$stage" -fs HFS+ -format UDZO -ov "$out" >/dev/null
echo "$out"
