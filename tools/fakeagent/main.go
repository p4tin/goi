// Command fakeagent is the scripted ACP agent from internal/acp/acptest, for
// E2E runs (set ai.agents.fake to its path).
package main

import "github.com/p4tin/goi/internal/acp/acptest"

func main() { acptest.Main() }
