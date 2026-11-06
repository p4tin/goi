package vfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// OpenMode tells the editor how much to do with a file.
type OpenMode string

const (
	ModeNormal      OpenMode = "normal"
	ModeNoHighlight OpenMode = "noHighlight"
	ModeReadOnly    OpenMode = "readOnly"
)

// Limits are the size thresholds (bytes) above which highlighting is
// disabled and above which files open read-only.
type Limits struct {
	HighlightBytes int64
	OpenBytes      int64
}

// DefaultLimits matches editor.maxHighlightSizeKB / editor.maxOpenSizeKB.
var DefaultLimits = Limits{HighlightBytes: 2500 * 1024, OpenBytes: 20000 * 1024}

// FileMeta describes a text file's on-disk form so it can be written back
// byte-for-byte (except for the edited content).
type FileMeta struct {
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"modTime"`
	Binary       bool      `json:"binary"`
	EOL          string    `json:"eol"`
	BOM          bool      `json:"bom"`
	FinalNewline bool      `json:"finalNewline"`
	Mode         OpenMode  `json:"mode"`
}

const binarySniffBytes = 8 << 10

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ErrNotWritable is returned by WriteText for binary or read-only files.
var ErrNotWritable = errors.New("file is not writable in the editor")

// ReadText reads path, detecting binary content, BOM and line endings. The
// returned content is normalized to "\n" line endings with the BOM removed.
func ReadText(path string, lim Limits) (FileMeta, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileMeta{}, "", fmt.Errorf("vfs: read: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return FileMeta{}, "", fmt.Errorf("vfs: read: %w", err)
	}
	if st.IsDir() {
		return FileMeta{}, "", fmt.Errorf("vfs: read: %s is a directory", path)
	}
	meta := FileMeta{Path: path, Size: st.Size(), ModTime: st.ModTime(), EOL: "\n", Mode: modeFor(st.Size(), lim)}

	head := make([]byte, binarySniffBytes)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return FileMeta{}, "", fmt.Errorf("vfs: read: %w", err)
	}
	if bytes.IndexByte(head[:n], 0) >= 0 {
		meta.Binary = true
		return meta, "", nil
	}
	rest, err := io.ReadAll(f)
	if err != nil {
		return FileMeta{}, "", fmt.Errorf("vfs: read: %w", err)
	}
	data := append(head[:n], rest...)

	if bytes.HasPrefix(data, utf8BOM) {
		meta.BOM = true
		data = data[len(utf8BOM):]
	}
	meta.EOL = detectEOL(data)
	meta.FinalNewline = len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r')
	return meta, normalizeEOL(string(data)), nil
}

func modeFor(size int64, lim Limits) OpenMode {
	switch {
	case lim.OpenBytes > 0 && size > lim.OpenBytes:
		return ModeReadOnly
	case lim.HighlightBytes > 0 && size > lim.HighlightBytes:
		return ModeNoHighlight
	default:
		return ModeNormal
	}
}

// detectEOL returns the majority line ending; ties and no endings give "\n".
func detectEOL(b []byte) string {
	var crlf, lf, cr int
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\r':
			if i+1 < len(b) && b[i+1] == '\n' {
				crlf++
				i++
			} else {
				cr++
			}
		case '\n':
			lf++
		}
	}
	switch {
	case crlf > lf && crlf > cr:
		return "\r\n"
	case cr > lf && cr > crlf:
		return "\r"
	default:
		return "\n"
	}
}

func normalizeEOL(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// WriteText atomically writes "\n"-normalized content to path, restoring the
// line endings and BOM recorded in meta. It returns refreshed metadata.
func WriteText(path, content string, meta FileMeta) (FileMeta, error) {
	if meta.Binary || meta.Mode == ModeReadOnly {
		return FileMeta{}, fmt.Errorf("vfs: write %s: %w", path, ErrNotWritable)
	}
	eol := meta.EOL
	if eol == "" {
		eol = "\n"
	}
	out := content
	if eol != "\n" {
		out = strings.ReplaceAll(content, "\n", eol)
	}
	data := []byte(out)
	if meta.BOM {
		data = append(append([]byte{}, utf8BOM...), data...)
	}
	if err := atomicWrite(path, data); err != nil {
		return FileMeta{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return FileMeta{}, fmt.Errorf("vfs: write: %w", err)
	}
	meta.Path = path
	meta.Size = st.Size()
	meta.ModTime = st.ModTime()
	meta.EOL = eol
	meta.FinalNewline = strings.HasSuffix(content, "\n")
	return meta, nil
}
