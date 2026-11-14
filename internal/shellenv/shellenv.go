// Package shellenv gives a GUI-launched Goi the PATH of the user's login
// shell. macOS starts apps from the Dock, Finder or Spotlight with launchd's
// PATH (/usr/bin:/bin:/usr/sbin:/sbin), without what ~/.zprofile or ~/.zshrc
// add (Homebrew, nvm, gvm…), so tools like npm and node aren't found.
package shellenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	startMark = "__GOI_PATH_START__"
	endMark   = "__GOI_PATH_END__"
)

// printPath works in sh, bash, zsh, fish and tcsh. The markers separate
// PATH from whatever the startup files print.
const printPath = `printf '%s' "` + startMark + `"; printf '%s' "$PATH"; printf '%s' "` + endMark + `"`

// LoginPath returns PATH as the user's interactive login shell sets it.
func LoginPath(ctx context.Context, shell string) (string, error) {
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", printPath)
	cmd.Stdin = nil
	// Startup files may leave background processes holding stdout.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("shellenv: %s: %w", shell, ctx.Err())
	}
	if err != nil {
		return "", fmt.Errorf("shellenv: %s: %w", shell, err)
	}
	i := bytes.LastIndex(out, []byte(startMark))
	j := bytes.LastIndex(out, []byte(endMark))
	if i < 0 || j < i {
		return "", errors.New("shellenv: no PATH in the login shell's output")
	}
	return string(out[i+len(startMark) : j]), nil
}

// Merge returns the login PATH followed by the current entries it lacks,
// without empty or duplicate entries. Both are POSIX PATHs (":"-separated):
// the fix only runs on macOS and Linux.
func Merge(current, login string) string {
	seen := map[string]bool{}
	var out []string
	for _, list := range []string{login, current} {
		for _, p := range strings.Split(list, ":") {
			if p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return strings.Join(out, ":")
}

// Fix merges the login shell's PATH into this process's PATH. On error PATH
// is left as it was.
func Fix(ctx context.Context, shell string) error {
	login, err := LoginPath(ctx, shell)
	if err != nil {
		return err
	}
	return os.Setenv("PATH", Merge(os.Getenv("PATH"), login))
}

// Shell is the user's login shell: $SHELL, else /bin/zsh on macOS and
// /bin/sh elsewhere.
func Shell(goos string) string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	if goos == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/sh"
}

// Needed reports whether Goi should import the login PATH: on macOS and
// Linux when it wasn't started from a terminal (which already passes the
// shell's environment and sets TERM).
func Needed(goos string) bool {
	return (goos == "darwin" || goos == "linux") && os.Getenv("TERM") == ""
}
