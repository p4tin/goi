// Package python is the Python language pack: project detection, the
// project interpreter and virtual environment, basedpyright and the Python
// run configuration. apps/pyi combines it with Pyi's identity.
package python

import (
	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/python/pyenv"
	"github.com/p4tin/goi/packs/python/pyright"
	"github.com/p4tin/goi/packs/python/pyrun"
)

// SettingsDefaults are the [python] defaults and Python-friendly project
// excludes.
const SettingsDefaults = `
[python]
interpreter = ""            # empty: the project's .venv, then python3 on PATH

[python.lsp]
enabled = true
path = ""                   # basedpyright-langserver; empty: PATH, then the tools folder
analysis = {}               # basedpyright.analysis settings

[project]
excludes = [".git", "node_modules", ".idea", ".pyi/cache", ".venv", "venv", "__pycache__", ".mypy_cache", ".pytest_cache", ".ruff_cache", ".tox", "*.egg-info", "build", "dist"]

[save]
format = "none"             # no Python formatter yet
`

// Pack returns the Python capabilities with identity id.
func Pack(id pack.Identity) pack.Pack {
	return pack.Pack{
		Identity:         id,
		Project:          pyenv.Detector{},
		Toolchain:        pyenv.Toolchain{},
		Server:           pyright.Server{},
		Run:              pyrun.Run{},
		SettingsDefaults: []byte(SettingsDefaults),
	}
}
