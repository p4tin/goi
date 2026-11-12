#!/bin/sh
# Prints the product version: info.version from the Wails v3 build config
# (build/config.yml, or build-v3/config.yml while the v3 scaffold is staged),
# falling back to wails.json's info.productVersion (Wails v2).
# With a tag argument (v1.2.3 or refs/tags/v1.2.3) it fails unless they match.
set -e
cd "$(dirname "$0")/.."

# yaml_version FILE prints the value of `version:` inside the top-level `info:` block.
yaml_version() {
  awk '
    /^[^[:space:]#]/ { in_info = ($0 ~ /^info:[[:space:]]*$/) ; next }
    in_info && /^[[:space:]]+version:/ {
      sub(/^[[:space:]]+version:[[:space:]]*/, "")
      sub(/[[:space:]]+#.*$/, "")
      gsub(/["'"'"']/, "")
      print
      exit
    }
  ' "$1"
}

v=""
for cfg in build/config.yml build-v3/config.yml; do
  if [ -f "$cfg" ]; then
    v=$(yaml_version "$cfg")
    src="$cfg info.version"
    [ -n "$v" ] && break
  fi
done
if [ -z "$v" ]; then
  v=$(node -p "require('./wails.json').info.productVersion")
  src="wails.json productVersion"
fi
if [ -z "$v" ]; then
  echo "no product version found" >&2
  exit 1
fi

if [ -n "$1" ]; then
  tag="${1#refs/tags/}"
  if [ "$tag" != "v$v" ]; then
    echo "tag $tag does not match $src $v" >&2
    exit 1
  fi
fi
echo "$v"
