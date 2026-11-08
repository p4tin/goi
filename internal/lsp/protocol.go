// Package lsp implements a Language Server Protocol client: JSON-RPC 2.0
// framing over a byte stream and the subset of protocol types Goi uses.
//
// Union types from the specification are normalized while decoding, so
// callers see one shape: markup is always a Markdown string, completion
// results are always a list, code-action results are always CodeActions.
package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Position is a zero-based line and UTF-16 character offset.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a half-open span between two positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a range in a document.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// locationLink is the alternative shape servers may return for definitions.
type locationLink struct {
	TargetURI            string `json:"targetUri"`
	TargetRange          Range  `json:"targetRange"`
	TargetSelectionRange Range  `json:"targetSelectionRange"`
}

// Locations decodes Location | Location[] | LocationLink[] | null.
type Locations []Location

// UnmarshalJSON implements json.Unmarshaler.
func (l *Locations) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*l = Locations{}
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '{' {
		var one Location
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*l = Locations{one}
		return nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, r := range raw {
		var probe struct {
			URI       string `json:"uri"`
			TargetURI string `json:"targetUri"`
		}
		if err := json.Unmarshal(r, &probe); err != nil {
			return err
		}
		if probe.TargetURI != "" {
			var ll locationLink
			if err := json.Unmarshal(r, &ll); err != nil {
				return err
			}
			*l = append(*l, Location{URI: ll.TargetURI, Range: ll.TargetSelectionRange})
			continue
		}
		var loc Location
		if err := json.Unmarshal(r, &loc); err != nil {
			return err
		}
		*l = append(*l, loc)
	}
	return nil
}

// TextEdit replaces Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// TextDocumentIdentifier names a document.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier names a document at a version.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version *int   `json:"version"`
}

// TextDocumentItem is a document sent with didOpen.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentPositionParams addresses a position in a document.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// TextDocumentEdit is a set of edits to one document.
type TextDocumentEdit struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                      `json:"edits"`
}

// WorkspaceEdit is a set of edits across documents. File create, rename and
// delete operations in documentChanges are dropped while decoding; gopls
// only emits them for refactorings Goi does not offer yet.
type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []TextDocumentEdit    `json:"documentChanges,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (w *WorkspaceEdit) UnmarshalJSON(b []byte) error {
	var raw struct {
		Changes         map[string][]TextEdit `json:"changes"`
		DocumentChanges []json.RawMessage     `json:"documentChanges"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	w.Changes = raw.Changes
	w.DocumentChanges = nil
	for _, r := range raw.DocumentChanges {
		var probe struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(r, &probe); err != nil {
			return err
		}
		if probe.Kind != "" {
			continue
		}
		var e TextDocumentEdit
		if err := json.Unmarshal(r, &e); err != nil {
			return err
		}
		w.DocumentChanges = append(w.DocumentChanges, e)
	}
	return nil
}

// Markup is documentation text normalized to Markdown. It decodes string,
// MarkupContent, MarkedString and MarkedString[] and encodes as a string.
type Markup string

// UnmarshalJSON implements json.Unmarshaler.
func (m *Markup) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*m = ""
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*m = Markup(s)
	case '{':
		var o struct {
			Kind     string `json:"kind"`
			Language string `json:"language"`
			Value    string `json:"value"`
		}
		if err := json.Unmarshal(b, &o); err != nil {
			return err
		}
		switch {
		case o.Language != "":
			*m = Markup("```" + o.Language + "\n" + o.Value + "\n```")
		case o.Kind == "plaintext":
			*m = Markup(escapeMarkdown(o.Value))
		default:
			*m = Markup(o.Value)
		}
	case '[':
		var parts []Markup
		if err := json.Unmarshal(b, &parts); err != nil {
			return err
		}
		var buf bytes.Buffer
		for i, p := range parts {
			if i > 0 {
				buf.WriteString("\n\n")
			}
			buf.WriteString(string(p))
		}
		*m = Markup(buf.String())
	default:
		return fmt.Errorf("lsp: unexpected markup %s", b)
	}
	return nil
}

func escapeMarkdown(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		switch r {
		case '\\', '`', '*', '_', '[', ']', '<', '>', '#':
			buf.WriteByte('\\')
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

// Diagnostic severities.
const (
	SeverityError       = 1
	SeverityWarning     = 2
	SeverityInformation = 3
	SeverityHint        = 4
)

// Diagnostic is a problem reported for a range. Code and Data are kept
// opaque so they round-trip to the server unchanged in code-action requests.
type Diagnostic struct {
	Range              Range                          `json:"range"`
	Severity           int                            `json:"severity,omitempty"`
	Code               any                            `json:"code,omitempty"`
	Source             string                         `json:"source,omitempty"`
	Message            string                         `json:"message"`
	Tags               []int                          `json:"tags,omitempty"`
	RelatedInformation []DiagnosticRelatedInformation `json:"relatedInformation,omitempty"`
	Data               any                            `json:"data,omitempty"`
}

// DiagnosticRelatedInformation points at a related location.
type DiagnosticRelatedInformation struct {
	Location Location `json:"location"`
	Message  string   `json:"message"`
}

// PublishDiagnosticsParams is the payload of textDocument/publishDiagnostics.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Insert text formats.
const (
	InsertTextPlain   = 1
	InsertTextSnippet = 2
)

// CompletionItem is one completion candidate.
type CompletionItem struct {
	Label               string     `json:"label"`
	Kind                int        `json:"kind,omitempty"`
	Detail              string     `json:"detail,omitempty"`
	Documentation       Markup     `json:"documentation,omitempty"`
	Deprecated          bool       `json:"deprecated,omitempty"`
	Tags                []int      `json:"tags,omitempty"`
	Preselect           bool       `json:"preselect,omitempty"`
	SortText            string     `json:"sortText,omitempty"`
	FilterText          string     `json:"filterText,omitempty"`
	InsertText          string     `json:"insertText,omitempty"`
	InsertTextFormat    int        `json:"insertTextFormat,omitempty"`
	TextEdit            *TextEdit  `json:"textEdit,omitempty"`
	AdditionalTextEdits []TextEdit `json:"additionalTextEdits,omitempty"`
	Command             *Command   `json:"command,omitempty"`
}

// CompletionList decodes CompletionItem[] | CompletionList | null.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *CompletionList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*c = CompletionList{Items: []CompletionItem{}}
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		return json.Unmarshal(b, &c.Items)
	}
	type plain CompletionList
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*c = CompletionList(p)
	if c.Items == nil {
		c.Items = []CompletionItem{}
	}
	return nil
}

// Hover is the result of textDocument/hover.
type Hover struct {
	Contents Markup `json:"contents"`
	Range    *Range `json:"range,omitempty"`
}

// SignatureHelp is the result of textDocument/signatureHelp.
type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

// SignatureInformation is one callable signature.
type SignatureInformation struct {
	Label           string                 `json:"label"`
	Documentation   Markup                 `json:"documentation,omitempty"`
	Parameters      []ParameterInformation `json:"parameters"`
	ActiveParameter *int                   `json:"activeParameter,omitempty"`
}

// ParameterInformation is a parameter of a signature. The protocol's label
// is either a substring or [start, end] UTF-16 offsets into the signature
// label; both decode to Label plus Start/End (-1 when given as text).
type ParameterInformation struct {
	Label         string `json:"label"`
	Start         int    `json:"start"`
	End           int    `json:"end"`
	Documentation Markup `json:"documentation,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *ParameterInformation) UnmarshalJSON(b []byte) error {
	var raw struct {
		Label         json.RawMessage `json:"label"`
		Documentation Markup          `json:"documentation"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	p.Documentation = raw.Documentation
	p.Start, p.End = -1, -1
	lb := bytes.TrimSpace(raw.Label)
	if len(lb) > 0 && lb[0] == '[' {
		var off [2]int
		if err := json.Unmarshal(lb, &off); err != nil {
			return err
		}
		p.Start, p.End = off[0], off[1]
		p.Label = ""
		return nil
	}
	return json.Unmarshal(lb, &p.Label)
}

// Symbol kinds used by Goi's UI (subset of the protocol's SymbolKind).
const (
	SymbolFile        = 1
	SymbolModule      = 2
	SymbolNamespace   = 3
	SymbolPackage     = 4
	SymbolClass       = 5
	SymbolMethod      = 6
	SymbolProperty    = 7
	SymbolField       = 8
	SymbolConstructor = 9
	SymbolEnum        = 10
	SymbolInterface   = 11
	SymbolFunction    = 12
	SymbolVariable    = 13
	SymbolConstant    = 14
	SymbolStruct      = 23
	SymbolTypeParam   = 26
)

// DocumentSymbol is a hierarchical symbol in a document.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolInformation is a flat symbol with a location (workspace/symbol).
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// Document highlight kinds.
const (
	HighlightText  = 1
	HighlightRead  = 2
	HighlightWrite = 3
)

// DocumentHighlight is an occurrence of the symbol under the cursor.
type DocumentHighlight struct {
	Range Range `json:"range"`
	Kind  int   `json:"kind,omitempty"`
}

// Command is a server command reference.
type Command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

// CodeAction is a quick-fix, refactoring or source action.
type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
	Disabled    *Disabled      `json:"disabled,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command       `json:"command,omitempty"`
	Data        any            `json:"data,omitempty"`
}

// Disabled explains why a code action cannot run.
type Disabled struct {
	Reason string `json:"reason"`
}

// CodeActions decodes (Command | CodeAction)[] | null; bare commands are
// wrapped in a CodeAction with the command's title.
type CodeActions []CodeAction

// UnmarshalJSON implements json.Unmarshaler.
func (c *CodeActions) UnmarshalJSON(b []byte) error {
	*c = CodeActions{}
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, r := range raw {
		var probe struct {
			Command json.RawMessage `json:"command"`
		}
		if err := json.Unmarshal(r, &probe); err != nil {
			return err
		}
		if p := bytes.TrimSpace(probe.Command); len(p) > 0 && p[0] == '"' {
			var cmd Command
			if err := json.Unmarshal(r, &cmd); err != nil {
				return err
			}
			*c = append(*c, CodeAction{Title: cmd.Title, Command: &cmd})
			continue
		}
		var a CodeAction
		if err := json.Unmarshal(r, &a); err != nil {
			return err
		}
		*c = append(*c, a)
	}
	return nil
}

// CodeActionContext carries the diagnostics a code action request is for.
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
	TriggerKind int          `json:"triggerKind,omitempty"`
}

// CodeActionParams is the payload of textDocument/codeAction.
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// PrepareRenameResult is the normalized result of textDocument/prepareRename.
// OK is false when the server refused (null result).
type PrepareRenameResult struct {
	OK          bool   `json:"ok"`
	Range       Range  `json:"range"`
	Placeholder string `json:"placeholder"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *PrepareRenameResult) UnmarshalJSON(b []byte) error {
	*p = PrepareRenameResult{}
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var raw struct {
		Start           *Position `json:"start"`
		End             *Position `json:"end"`
		Range           *Range    `json:"range"`
		Placeholder     string    `json:"placeholder"`
		DefaultBehavior bool      `json:"defaultBehavior"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	switch {
	case raw.Range != nil:
		p.OK, p.Range, p.Placeholder = true, *raw.Range, raw.Placeholder
	case raw.Start != nil && raw.End != nil:
		p.OK, p.Range = true, Range{Start: *raw.Start, End: *raw.End}
	case raw.DefaultBehavior:
		p.OK = true
	}
	return nil
}

// File change types for workspace/didChangeWatchedFiles.
const (
	FileCreated = 1
	FileChanged = 2
	FileDeleted = 3
)

// FileEvent is one watched-file change.
type FileEvent struct {
	URI  string `json:"uri"`
	Type int    `json:"type"`
}

// ProgressParams is the payload of $/progress.
type ProgressParams struct {
	Token any             `json:"token"`
	Value json.RawMessage `json:"value"`
}

// WorkDoneProgress is the value of a work-done progress notification.
type WorkDoneProgress struct {
	Kind       string `json:"kind"` // begin | report | end
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Percentage *int   `json:"percentage,omitempty"`
}

// ConfigurationItem is one section requested by workspace/configuration.
type ConfigurationItem struct {
	ScopeURI string `json:"scopeUri,omitempty"`
	Section  string `json:"section,omitempty"`
}

// ConfigurationParams is the payload of workspace/configuration.
type ConfigurationParams struct {
	Items []ConfigurationItem `json:"items"`
}

// ApplyWorkspaceEditParams is the payload of workspace/applyEdit.
type ApplyWorkspaceEditParams struct {
	Label string        `json:"label,omitempty"`
	Edit  WorkspaceEdit `json:"edit"`
}

// ApplyWorkspaceEditResult answers workspace/applyEdit.
type ApplyWorkspaceEditResult struct {
	Applied       bool   `json:"applied"`
	FailureReason string `json:"failureReason,omitempty"`
}

// MessageParams is the payload of window/logMessage and window/showMessage.
type MessageParams struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}
