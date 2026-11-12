#!/bin/sh
# Prints the product version: info.version from the Wails build config
# (build/config.yml).
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
cfg=build/config.yml
if [ -f "$cfg" ]; then
  v=$(yaml_version "$cfg")
  src="$cfg info.version"
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
