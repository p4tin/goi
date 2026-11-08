#!/bin/sh
# Prints the product version from wails.json (the single source of truth).
# With a tag argument (v1.2.3 or refs/tags/v1.2.3) it fails unless they match.
set -e
cd "$(dirname "$0")/.."
v=$(node -p "require('./wails.json').info.productVersion")
if [ -n "$1" ]; then
  tag="${1#refs/tags/}"
  if [ "$tag" != "v$v" ]; then
    echo "tag $tag does not match wails.json productVersion $v" >&2
    exit 1
  fi
fi
echo "$v"
