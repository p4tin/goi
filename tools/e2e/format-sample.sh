#!/bin/sh
# Creates the project and Goi profile used by format.json:
#   tools/e2e/format-sample.sh <dir>
# <dir>/proj holds JSON, JSONC and YAML fixtures; <dir>/home is a GOI_HOME
# with 2-space indentation and JSON format on save.
set -e
d="${1:?usage: format-sample.sh <dir>}"
rm -rf "$d"
mkdir -p "$d/proj" "$d/home/config"
printf 'module example.com/formatsample\n\ngo 1.22\n' > "$d/proj/go.mod"
printf '%s' '{"a":1,"b":[1,2,{"c":12345678901234567890}],"d":{}}' > "$d/proj/data.json"
printf '%s' '{"a": }' > "$d/proj/broken.json"
printf '%s' '{"x":1}' > "$d/proj/onsave.json"
cat > "$d/proj/settings.jsonc" <<'JSON'
// editor settings
{"tabs": true, // trailing note
  /* block */ "size": 4,
  "list": [1, 2,] // after list
}
JSON
cat > "$d/proj/app.yaml" <<'YAML'
# app config
defaults: &defaults
    adapter: postgres   # db
    script: |
        echo one
          two
dev:
    <<: *defaults
    hosts:
    - a
    - b
YAML
printf 'enabled: yes\nversion: 1.10\n' > "$d/proj/values.yaml"
printf 'a: [1, 2\n' > "$d/proj/bad.yaml"
cat > "$d/home/config/settings.toml" <<'TOML'
[editor]
tabSize = 2

[json]
formatOnSave = true
TOML
echo "sample ready: $d/proj (GOI_HOME=$d/home)"
