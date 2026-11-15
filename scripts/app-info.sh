#!/bin/sh
# Prints a value from the `info:` block of an app's build config:
#   scripts/app-info.sh <app> <key>      e.g. scripts/app-info.sh goi productName
set -e
cd "$(dirname "$0")/.."
app="${1:?usage: app-info.sh <app> <key>}"
key="${2:?usage: app-info.sh <app> <key>}"
cfg="apps/$app/build/config.yml"
[ -f "$cfg" ] || { echo "app-info: no $cfg" >&2; exit 1; }
awk -v key="$key" '
  /^[^[:space:]#]/ { in_info = ($0 ~ /^info:[[:space:]]*$/) ; next }
  in_info && $0 ~ "^[[:space:]]+" key ":" {
    sub("^[[:space:]]+" key ":[[:space:]]*", "")
    sub(/[[:space:]]+#.*$/, "")
    gsub(/["'"'"']/, "")
    print
    exit
  }
' "$cfg"
