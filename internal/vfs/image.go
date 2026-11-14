package vfs

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotImage is returned by ReadDataURL for files without an image extension.
var ErrNotImage = errors.New("not an image file")

// ErrTooLarge is returned by ReadDataURL for files over the size limit.
var ErrTooLarge = errors.New("file is too large")

// imageTypes maps the image extensions the Markdown preview shows to their
// media types. Content is never sniffed.
var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".svg":  "image/svg+xml",
}

// ReadDataURL returns an image file as a base64 data URL.
func ReadDataURL(path string, maxBytes int64) (string, error) {
	mime, ok := imageTypes[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return "", ErrNotImage
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", ErrNotImage
	}
	if fi.Size() > maxBytes {
		return "", fmt.Errorf("%s: %w", path, ErrTooLarge)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if int64(len(b)) > maxBytes {
		return "", fmt.Errorf("%s: %w", path, ErrTooLarge)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}
