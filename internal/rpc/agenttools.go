package rpc

import (
	"encoding/json"

	"github.com/p4tin/goi/internal/mcp"
)

// mcpInstructions is sent to MCP clients at initialize.
const mcpInstructions = "Tools of the Goi IDE window that has this project open. Paths are absolute OS paths; lines and columns are 1-based. " +
	"read_file sees unsaved editor changes. replace_text and write_file edit through the editor (one undoable change, then saved) and the user can review and revert them. " +
	"Prefer get_diagnostics after edits, and run_tests or run_configuration to verify."

type prop struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// schema builds an object JSON schema. Every tool also takes an optional
// "project" (external clients with several windows open).
func schema(required []string, props map[string]prop) json.RawMessage {
	all := map[string]prop{"project": {Type: "string", Description: "Project folder of the Goi window to use (only needed when several are open)."}}
	for k, v := range props {
		all[k] = v
	}
	if required == nil {
		required = []string{}
	}
	b, _ := json.Marshal(map[string]any{"type": "object", "properties": all, "required": required, "additionalProperties": false})
	return b
}

var (
	pPath   = prop{Type: "string", Description: "Absolute file path."}
	pLine   = prop{Type: "integer", Description: "1-based line."}
	pColumn = prop{Type: "integer", Description: "1-based column (character in the line)."}
	pTime   = prop{Type: "integer", Description: "Give up waiting after this many seconds (default 300)."}
)

// agentTools are the IDE tools served over MCP. Their implementations are in
// frontend/src/agent/tools.ts (keyed by name).
func agentTools() []mcp.Tool {
	return []mcp.Tool{
		{Name: "get_open_editors", Description: "List the files open in editor tabs, with which one is active and which have unsaved changes.", InputSchema: schema(nil, nil)},
		{Name: "get_active_editor", Description: "The active editor's file, cursor position and selected text.", InputSchema: schema(nil, nil)},
		{Name: "open_file", Description: "Open a file in the editor, optionally at a line and column, and focus it.", InputSchema: schema([]string{"path"}, map[string]prop{"path": pPath, "line": pLine, "column": pColumn})},
		{Name: "read_file", Description: "Read a file's text as the editor sees it (including unsaved changes), optionally a 1-based inclusive line range.", InputSchema: schema([]string{"path"}, map[string]prop{"path": pPath, "startLine": pLine, "endLine": pLine})},
		{Name: "replace_text", Description: "Replace exact text in a file through the editor. oldText must occur exactly once unless replaceAll is true. The change is saved and recorded for review.", InputSchema: schema([]string{"path", "oldText", "newText"}, map[string]prop{
			"path": pPath, "oldText": {Type: "string", Description: "Exact text to replace."}, "newText": {Type: "string", Description: "Replacement text."},
			"replaceAll": {Type: "boolean", Description: "Replace every occurrence."},
		})},
		{Name: "write_file", Description: "Create or overwrite a file with content through the editor. The change is saved and recorded for review.", InputSchema: schema([]string{"path", "content"}, map[string]prop{"path": pPath, "content": {Type: "string", Description: "Full new file content."}})},
		{Name: "get_diagnostics", Description: "Compiler, gopls and linter problems, for one file or the whole project.", InputSchema: schema(nil, map[string]prop{
			"path":     {Type: "string", Description: "Absolute file path; omit for every file."},
			"severity": {Type: "string", Description: "Minimum severity (default warning).", Enum: []string{"error", "warning", "all"}},
		})},
		{Name: "find_symbol", Description: "Search Go symbols (types, functions, methods…) in the workspace by name.", InputSchema: schema([]string{"query"}, map[string]prop{"query": {Type: "string", Description: "Symbol name or fuzzy query."}})},
		{Name: "find_references", Description: "Find usages of the symbol at a position.", InputSchema: schema([]string{"path", "line", "column"}, map[string]prop{"path": pPath, "line": pLine, "column": pColumn})},
		{Name: "go_to_definition", Description: "Find where the symbol at a position is defined.", InputSchema: schema([]string{"path", "line", "column"}, map[string]prop{"path": pPath, "line": pLine, "column": pColumn})},
		{Name: "search_in_project", Description: "Search text in the project's files (respects excludes; sees unsaved editor changes).", InputSchema: schema([]string{"pattern"}, map[string]prop{
			"pattern":       {Type: "string", Description: "Text or regular expression."},
			"regex":         {Type: "boolean", Description: "Treat pattern as a Go regular expression."},
			"caseSensitive": {Type: "boolean"},
			"fileMask":      {Type: "string", Description: "Comma-separated file name globs, e.g. \"*.go,!*_test.go\"."},
			"maxResults":    {Type: "integer", Description: "Default 100."},
		})},
		{Name: "list_run_configurations", Description: "List the project's run configurations (name and type).", InputSchema: schema(nil, nil)},
		{Name: "run_configuration", Description: "Run a run configuration in the IDE's Run window, wait for it to finish, and return its exit code and the end of its output.", InputSchema: schema([]string{"name"}, map[string]prop{"name": {Type: "string", Description: "Run configuration name."}, "timeoutSeconds": pTime})},
		{Name: "run_tests", Description: "Run Go tests in the IDE's test runner and return the result summary and failures.", InputSchema: schema(nil, map[string]prop{
			"package":        {Type: "string", Description: "Package pattern relative to the project root, e.g. ./... (default) or ./internal/foo."},
			"test":           {Type: "string", Description: "Test name regular expression (-run)."},
			"timeoutSeconds": pTime,
		})},
		{Name: "format_file", Description: "Format a Go file with the project's formatter (goimports or gofmt) through the editor, and save it.", InputSchema: schema([]string{"path"}, map[string]prop{"path": pPath})},
		{Name: "git_status", Description: "The repository's current branch and changed files.", InputSchema: schema(nil, nil)},
	}
}
