// Package lint holds the lint issue model the core shows as diagnostics.
// The app pack's Linter (Goi: golangci-lint) produces the issues.
package lint

// Pos is a 0-based line and UTF-16 character, like LSP.
type Pos struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Edit replaces [Start, End) with NewText.
type Edit struct {
	Start   Pos    `json:"start"`
	End     Pos    `json:"end"`
	NewText string `json:"newText"`
}

// Issue is one finding.
type Issue struct {
	Path     string `json:"path"`
	Start    Pos    `json:"start"`
	End      Pos    `json:"end"`
	Linter   string `json:"linter"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	// Fix holds the suggested edits (same file), if any.
	Fix []Edit `json:"fix"`
}
