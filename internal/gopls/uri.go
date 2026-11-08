// Package gopls runs and talks to gopls for one project: binary resolution
// and installation, process supervision, document synchronization and the
// language requests Goi uses. It reports to the host through callbacks and
// never imports UI code.
package gopls

import (
	"net/url"
	"runtime"
	"strings"
)

// PathToURI converts an absolute OS path to a file:// URI.
func PathToURI(path string) string {
	return pathToURI(path, runtime.GOOS == "windows")
}

func pathToURI(path string, windows bool) string {
	p := path
	if windows {
		p = strings.ReplaceAll(p, `\`, "/")
		if len(p) >= 2 && p[1] == ':' {
			p = "/" + strings.ToUpper(p[:1]) + p[1:]
		}
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String()
}

// URIToPath converts a file:// URI to an OS path. Non-file URIs are
// returned unchanged.
func URIToPath(uri string) string {
	return uriToPath(uri, runtime.GOOS == "windows")
}

func uriToPath(uri string, windows bool) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	p := u.Path
	if windows {
		if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			p = strings.ToUpper(p[1:2]) + p[2:]
		}
		return strings.ReplaceAll(p, "/", `\`)
	}
	// Not filepath.FromSlash: the result must follow the windows argument,
	// not the OS the code runs on (tests exercise both).
	return p
}
