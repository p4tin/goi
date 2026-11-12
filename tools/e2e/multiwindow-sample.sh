#!/bin/sh
# Creates the two modules and the Goi profile used by multiwindow.json:
#   tools/e2e/multiwindow-sample.sh <dir>
# <dir>/a and <dir>/b are projects; <dir>/home is a GOI_HOME whose recent list
# holds both, so the Welcome window can open b.
set -e
d="${1:?usage: multiwindow-sample.sh <dir>}"
for p in a b; do
  mkdir -p "$d/$p"
  printf 'module example.com/%s\n\ngo 1.22\n' "$p" > "$d/$p/go.mod"
  printf 'package main\n\nfunc main() {}\n' > "$d/$p/only_$p.go"
done
mkdir -p "$d/home/config"
cat > "$d/home/config/recent.json" <<JSON
[
  {"path": "$d/b", "name": "b", "kind": "module", "openedAt": "2026-01-02T00:00:00Z"},
  {"path": "$d/a", "name": "a", "kind": "module", "openedAt": "2026-01-01T00:00:00Z"}
]
JSON
echo "sample ready: $d (GOI_HOME=$d/home)"
