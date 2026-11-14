package shellenv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeShell writes a login shell that prints a banner (as startup files
// do), adds dir to PATH, then runs the -c command.
func fakeShell(t *testing.T, dir, body string) string {
	t.Helper()
	sh := filepath.Join(t.TempDir(), "fakesh")
	script := "#!/bin/sh\necho 'Welcome banner'\necho 'noise on stderr' >&2\nPATH=\"" + dir + ":$PATH\"\nexport PATH\n" + body
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return sh
}

// runC is the fake shell's body: run the last argument (the -c command).
const runC = "for last; do :; done\nexec /bin/sh -c \"$last\"\n"

func TestFixFindsToolsFromALoginShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no login shells on Windows")
	}
	// npm lives only in a folder the login shell adds (like /opt/homebrew/bin).
	brew := t.TempDir()
	os.WriteFile(filepath.Join(brew, "npm"), []byte("#!/bin/sh\n"), 0o755)
	// What launchd gives apps started from the Dock or Finder.
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	if _, err := exec.LookPath("npm"); err == nil {
		t.Skip("npm is in a system folder on this machine")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Fix(ctx, fakeShell(t, brew, runC)); err != nil {
		t.Fatal(err)
	}
	p, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("npm still not found; PATH=%s", os.Getenv("PATH"))
	}
	if filepath.Dir(p) != brew {
		t.Errorf("npm = %s", p)
	}
	if got := os.Getenv("PATH"); !strings.HasSuffix(got, "/usr/bin:/bin:/usr/sbin:/sbin") || !strings.HasPrefix(got, brew+":") {
		t.Errorf("PATH = %s", got)
	}
}

func TestLoginPathErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no login shells on Windows")
	}
	ctx := context.Background()
	if _, err := LoginPath(ctx, fakeShell(t, "/x", "exit 3\n")); err == nil {
		t.Error("failing shell: want error")
	}
	if _, err := LoginPath(ctx, fakeShell(t, "/x", "echo no markers\n")); err == nil {
		t.Error("no markers: want error")
	}
	tctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := LoginPath(tctx, fakeShell(t, "/x", "sleep 30\n")); err == nil {
		t.Error("hanging shell: want error")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("timeout took %v", d)
	}
	if _, err := LoginPath(ctx, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing shell: want error")
	}
}

func TestMerge(t *testing.T) {
	tests := []struct{ current, login, want string }{
		{"/usr/bin:/bin", "/opt/homebrew/bin:/usr/bin:/bin", "/opt/homebrew/bin:/usr/bin:/bin"},
		{"/app/extra:/usr/bin", "/opt/homebrew/bin:/usr/bin", "/opt/homebrew/bin:/usr/bin:/app/extra"},
		{"/usr/bin", "", "/usr/bin"},
		{"", "/a::/b:/a", "/a:/b"},
	}
	for _, tt := range tests {
		if got := Merge(tt.current, tt.login); got != tt.want {
			t.Errorf("Merge(%q, %q) = %q, want %q", tt.current, tt.login, got, tt.want)
		}
	}
}

// GOI_REAL_SHELL=1 checks the user's real login shell: npm must be found
// after Fix even when the test starts with launchd's PATH.
func TestRealShell(t *testing.T) {
	if os.Getenv("GOI_REAL_SHELL") == "" {
		t.Skip("set GOI_REAL_SHELL=1 to use the real login shell")
	}
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := Fix(ctx, Shell(runtime.GOOS)); err != nil {
		t.Fatal(err)
	}
	t.Logf("login PATH in %v: %s", time.Since(start).Round(time.Millisecond), os.Getenv("PATH"))
	for _, tool := range []string{"npm", "node"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Errorf("%s not found", tool)
			continue
		}
		t.Logf("%s = %s", tool, p)
	}
}
