#!/bin/sh
# Creates the project and Goi profile used by agent.json:
#   tools/e2e/agent-sample.sh <dir> <fakeagent>
# <dir>/proj is the project; <dir>/home is a GOI_HOME whose settings define
# the scripted ACP agent "fake" (build it: go build -o bin/fakeagent ./tools/fakeagent).
set -e
d="${1:?usage: agent-sample.sh <dir> <fakeagent>}"
agent="${2:?usage: agent-sample.sh <dir> <fakeagent>}"
rm -rf "$d"
mkdir -p "$d/proj" "$d/home/config"
printf 'module example.com/agentsample\n\ngo 1.22\n' > "$d/proj/go.mod"
printf 'package main\n\nimport "fmt"\n\nfunc main() {\n\tfmt.Println("hi")\n}\n' > "$d/proj/main.go"
cat > "$d/home/config/settings.toml" <<TOML
[ai]
defaultAgent = "fake"

[ai.agents]
fake = "$agent"
TOML
echo "sample ready: $d/proj (GOI_HOME=$d/home)"
