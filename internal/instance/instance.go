// Package instance ensures each project is open in at most one Goi window.
//
// The window owning a project holds a lock file
// <cacheDir>/instances/<hash>.lock containing "pid port token". It listens on
// 127.0.0.1:port for one-line commands authenticated by the token:
// "ping <token>" answers "goi-ok" and "focus <token>" brings the window
// forward. A lock whose pid is dead or whose port doesn't answer is stale and
// is replaced.
package instance

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const dialTimeout = 500 * time.Millisecond

// ErrHeld reports that another live process holds the project's lock.
type ErrHeld struct {
	Pid  int
	Port int
}

func (e *ErrHeld) Error() string {
	return fmt.Sprintf("instance: project is open in process %d", e.Pid)
}

// Lock is a held project lock.
type Lock struct {
	path    string
	token   string
	ln      net.Listener
	onFocus func()

	once sync.Once
	wg   sync.WaitGroup
}

type record struct {
	pid   int
	port  int
	token string
}

// LockPath returns the lock file for a project hash.
func LockPath(cacheDir, hash string) string {
	return filepath.Join(cacheDir, "instances", hash+".lock")
}

// Acquire takes the lock for hash. onFocus runs (on a listener goroutine)
// when another process asks this one to focus. If a live holder exists it
// returns *ErrHeld.
func Acquire(cacheDir, hash string, onFocus func()) (*Lock, error) {
	path := LockPath(cacheDir, hash)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("instance: %w", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("instance: listen: %w", err)
	}
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		ln.Close()
		return nil, fmt.Errorf("instance: token: %w", err)
	}
	l := &Lock{path: path, token: hex.EncodeToString(tok), ln: ln, onFocus: onFocus}
	content := fmt.Sprintf("%d %d %s\n", os.Getpid(), ln.Addr().(*net.TCPAddr).Port, l.token)

	for attempt := 0; attempt < 3; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, werr := f.WriteString(content)
			cerr := f.Close()
			if werr != nil || cerr != nil {
				os.Remove(path)
				ln.Close()
				return nil, fmt.Errorf("instance: write lock: %w", errors.Join(werr, cerr))
			}
			l.wg.Add(1)
			go l.serve()
			return l, nil
		}
		if !errors.Is(err, os.ErrExist) {
			ln.Close()
			return nil, fmt.Errorf("instance: create lock: %w", err)
		}
		if rec, ok := readRecord(path); ok && rec.pid != os.Getpid() && pidAlive(rec.pid) && ping(rec) {
			ln.Close()
			return nil, &ErrHeld{Pid: rec.pid, Port: rec.port}
		}
		// Stale: remove and retry.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			ln.Close()
			return nil, fmt.Errorf("instance: remove stale lock: %w", err)
		}
	}
	ln.Close()
	return nil, fmt.Errorf("instance: could not acquire %s", path)
}

// Held reports whether a live process holds the lock for hash.
func Held(cacheDir, hash string) bool {
	rec, ok := readRecord(LockPath(cacheDir, hash))
	return ok && rec.pid != os.Getpid() && pidAlive(rec.pid) && ping(rec)
}

// Focus asks the holder of hash's lock to bring its window forward.
func Focus(cacheDir, hash string) error {
	rec, ok := readRecord(LockPath(cacheDir, hash))
	if !ok {
		return errors.New("instance: no lock holder")
	}
	reply, err := send(rec.port, "focus "+rec.token)
	if err != nil {
		return fmt.Errorf("instance: focus: %w", err)
	}
	if reply != "ok" {
		return fmt.Errorf("instance: focus rejected: %q", reply)
	}
	return nil
}

// Release stops listening and removes the lock file if it is still ours.
func (l *Lock) Release() {
	l.once.Do(func() {
		l.ln.Close()
		l.wg.Wait()
		if rec, ok := readRecord(l.path); ok && rec.pid == os.Getpid() && rec.token == l.token {
			os.Remove(l.path)
		}
	})
}

func (l *Lock) serve() {
	defer l.wg.Done()
	for {
		c, err := l.ln.Accept()
		if err != nil {
			return
		}
		l.handle(c)
	}
}

func (l *Lock) handle(c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return
	}
	cmd, tok, _ := strings.Cut(strings.TrimSpace(line), " ")
	if tok != l.token {
		fmt.Fprint(c, "denied\n")
		return
	}
	switch cmd {
	case "ping":
		fmt.Fprint(c, "goi-ok\n")
	case "focus":
		if l.onFocus != nil {
			l.onFocus()
		}
		fmt.Fprint(c, "ok\n")
	default:
		fmt.Fprint(c, "unknown\n")
	}
}

func readRecord(path string) (record, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return record{}, false
	}
	f := strings.Fields(string(b))
	if len(f) != 3 {
		return record{}, false
	}
	pid, err1 := strconv.Atoi(f[0])
	port, err2 := strconv.Atoi(f[1])
	if err1 != nil || err2 != nil {
		return record{}, false
	}
	return record{pid: pid, port: port, token: f[2]}, true
}

func ping(rec record) bool {
	reply, err := send(rec.port, "ping "+rec.token)
	return err == nil && reply == "goi-ok"
}

func send(port int, line string) (string, error) {
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), dialTimeout)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprint(c, line+"\n"); err != nil {
		return "", err
	}
	reply, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(reply), nil
}
