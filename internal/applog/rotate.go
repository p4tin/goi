package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RotatingWriter is an io.WriteCloser that rolls its file over once it would
// exceed MaxBytes, keeping at most Keep previous files (path.1 … path.Keep).
type RotatingWriter struct {
	path     string
	maxBytes int64
	keep     int

	mu   sync.Mutex
	f    *os.File
	size int64
}

// NewRotatingWriter opens (appending) or creates path.
func NewRotatingWriter(path string, maxBytes int64, keep int) (*RotatingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("applog: mkdir: %w", err)
	}
	w := &RotatingWriter{path: path, maxBytes: maxBytes, keep: keep}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("applog: open: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("applog: stat: %w", err)
	}
	w.f, w.size = f, st.Size()
	return nil
}

// Write implements io.Writer.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingWriter) rotate() error {
	if err := w.f.Close(); err != nil {
		return fmt.Errorf("applog: close: %w", err)
	}
	os.Remove(fmt.Sprintf("%s.%d", w.path, w.keep))
	for i := w.keep - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", w.path, i), fmt.Sprintf("%s.%d", w.path, i+1))
	}
	if w.keep > 0 {
		if err := os.Rename(w.path, w.path+".1"); err != nil {
			return fmt.Errorf("applog: rotate: %w", err)
		}
	} else {
		os.Remove(w.path)
	}
	return w.open()
}

// Close implements io.Closer.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}
