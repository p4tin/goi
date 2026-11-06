package vfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/p4tin/goi/internal/events"
)

// Op is the kind of a file system change.
type Op string

const (
	OpCreate Op = "create"
	OpRemove Op = "remove"
	OpRename Op = "rename" // the path was moved away; the new name arrives as a create
	OpWrite  Op = "write"
)

// Event is one file system change under a watched root.
type Event struct {
	Path  string `json:"path"`
	Op    Op     `json:"op"`
	IsDir bool   `json:"isDir"`
}

// Batch is the payload of events.TopicFSChanged.
type Batch struct {
	Events []Event `json:"events"`
}

// Watcher reports changes under a set of roots, coalesced into batches, and
// can additionally watch individual files anywhere on disk.
type Watcher interface {
	// Subscribe registers fn to receive each coalesced batch.
	Subscribe(fn func([]Event)) (cancel func())
	AddRoot(root string) error
	RemoveRoot(root string) error
	// WatchFile calls fn (debounced) whenever path is created, written,
	// replaced or removed. path need not be under a root.
	WatchFile(path string, fn func()) (cancel func(), err error)
	// Close stops the watcher. No callbacks run after it returns. Callbacks
	// must not call Close.
	Close() error
}

const (
	defaultWindow   = 75 * time.Millisecond
	defaultDebounce = 100 * time.Millisecond
	tempPrefix      = ".goi-tmp-"
)

type pendingEvent struct {
	Event
	dropped bool
}

type fileWatch struct {
	path  string
	fn    func()
	timer *time.Timer
}

type watcher struct {
	fsw      *fsnotify.Watcher
	m        Matcher
	bus      events.Bus
	window   time.Duration
	debounce time.Duration

	mu            sync.Mutex
	closed        bool
	roots         []string
	treeDirs      map[string]string // watched directory -> owning root
	fileDirs      map[string]int    // directory -> number of file watches in it
	fileWatches   map[string]map[int]*fileWatch
	nextID        int
	subs          map[int]func([]Event)
	pending       []*pendingEvent
	pendingIdx    map[string]*pendingEvent
	timer         *time.Timer
	limitNotified bool

	deliverMu sync.Mutex // held while callbacks run; Close waits on it
	done      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

// NewWatcher watches every non-excluded directory under roots. bus may be
// nil; when set, each batch is also emitted as events.TopicFSChanged.
func NewWatcher(roots []string, m Matcher, bus events.Bus) (Watcher, error) {
	return newWatcher(roots, m, bus, defaultWindow)
}

func newWatcher(roots []string, m Matcher, bus events.Bus, window time.Duration) (*watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("vfs: watcher: %w", err)
	}
	if m == nil {
		m = MatchNone
	}
	if bus == nil {
		bus = events.Nop{}
	}
	w := &watcher{
		fsw:         fsw,
		m:           m,
		bus:         bus,
		window:      window,
		debounce:    defaultDebounce,
		treeDirs:    map[string]string{},
		fileDirs:    map[string]int{},
		fileWatches: map[string]map[int]*fileWatch{},
		subs:        map[int]func([]Event){},
		pendingIdx:  map[string]*pendingEvent{},
		done:        make(chan struct{}),
	}
	w.wg.Add(1)
	go w.loop()
	for _, r := range roots {
		if err := w.AddRoot(r); err != nil {
			w.Close()
			return nil, err
		}
	}
	return w, nil
}

func (w *watcher) Subscribe(fn func([]Event)) (cancel func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	id := w.nextID
	w.nextID++
	w.subs[id] = fn
	return func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		delete(w.subs, id)
	}
}

func (w *watcher) AddRoot(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("vfs: watch root: %w", err)
	}
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("vfs: watch root: %w", err)
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return errors.New("vfs: watcher closed")
	}
	for _, r := range w.roots {
		if r == root {
			w.mu.Unlock()
			return nil
		}
	}
	w.roots = append(w.roots, root)
	w.mu.Unlock()
	w.addTree(root, root, false)
	return nil
}

func (w *watcher) RemoveRoot(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("vfs: unwatch root: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, r := range w.roots {
		if r == root {
			w.roots = append(w.roots[:i], w.roots[i+1:]...)
			break
		}
	}
	for dir, owner := range w.treeDirs {
		if owner == root {
			w.unwatchTreeDirLocked(dir)
		}
	}
	return nil
}

func (w *watcher) WatchFile(path string, fn func()) (cancel func(), err error) {
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("vfs: watch file: %w", err)
	}
	dir := filepath.Dir(path)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, errors.New("vfs: watcher closed")
	}
	if w.fileDirs[dir] == 0 {
		if _, tree := w.treeDirs[dir]; !tree {
			if err := w.fsw.Add(dir); err != nil {
				w.limitCheckLocked(err)
				return nil, fmt.Errorf("vfs: watch file: %w", err)
			}
		}
	}
	w.fileDirs[dir]++
	if w.fileWatches[dir] == nil {
		w.fileWatches[dir] = map[int]*fileWatch{}
	}
	id := w.nextID
	w.nextID++
	fw := &fileWatch{path: path, fn: fn}
	w.fileWatches[dir][id] = fw
	var once sync.Once
	return func() {
		once.Do(func() {
			w.mu.Lock()
			defer w.mu.Unlock()
			if fw.timer != nil {
				fw.timer.Stop()
			}
			delete(w.fileWatches[dir], id)
			w.fileDirs[dir]--
			if w.fileDirs[dir] <= 0 {
				delete(w.fileDirs, dir)
				delete(w.fileWatches, dir)
				if _, tree := w.treeDirs[dir]; !tree && !w.closed {
					w.fsw.Remove(dir)
				}
			}
		})
	}, nil
}

func (w *watcher) Close() error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		if w.timer != nil {
			w.timer.Stop()
			w.timer = nil
		}
		for _, fws := range w.fileWatches {
			for _, fw := range fws {
				if fw.timer != nil {
					fw.timer.Stop()
				}
			}
		}
		w.mu.Unlock()
		close(w.done)
		w.closeErr = w.fsw.Close()
		w.wg.Wait()
		// Wait for any callback already running.
		w.deliverMu.Lock()
		w.deliverMu.Unlock()
	})
	return w.closeErr
}

func (w *watcher) loop() {
	defer w.wg.Done()
	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handle(ev)
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// Overflow and transient errors: nothing actionable; the next
			// events still flow.
		}
	}
}

func (w *watcher) handle(ev fsnotify.Event) {
	if ev.Op == fsnotify.Chmod || ev.Op == 0 {
		return
	}
	name := filepath.Clean(ev.Name)
	op := mapOp(ev.Op)

	w.fireFileWatches(name)

	if strings.HasPrefix(filepath.Base(name), tempPrefix) {
		return
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	root := w.rootForLocked(name)
	if root == "" || name == root {
		w.mu.Unlock()
		return
	}
	_, wasDir := w.treeDirs[name]
	w.mu.Unlock()

	isDir := wasDir
	if op == OpCreate || op == OpWrite {
		if st, err := os.Stat(name); err == nil {
			isDir = st.IsDir()
		} else if op == OpCreate {
			// Already gone again; nothing to report.
			return
		}
	}
	if w.m.Match(rel(root, name), isDir) {
		return
	}

	switch {
	case op == OpCreate && isDir:
		w.enqueue(Event{Path: name, Op: OpCreate, IsDir: true})
		w.addTree(name, root, true)
		return
	case (op == OpRemove || op == OpRename) && wasDir:
		w.mu.Lock()
		prefix := name + string(filepath.Separator)
		for dir := range w.treeDirs {
			if dir == name || strings.HasPrefix(dir, prefix) {
				w.unwatchTreeDirLocked(dir)
			}
		}
		w.mu.Unlock()
	}
	w.enqueue(Event{Path: name, Op: op, IsDir: isDir})
}

func mapOp(op fsnotify.Op) Op {
	switch {
	case op.Has(fsnotify.Remove):
		return OpRemove
	case op.Has(fsnotify.Rename):
		return OpRename
	case op.Has(fsnotify.Create):
		return OpCreate
	default:
		return OpWrite
	}
}

func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(r)
}

// rootForLocked returns the deepest root containing path, or "".
func (w *watcher) rootForLocked(path string) string {
	best := ""
	for _, r := range w.roots {
		if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
			if len(r) > len(best) {
				best = r
			}
		}
	}
	return best
}

// addTree watches dir and its non-excluded subdirectories. With synth, it
// also reports every entry found beneath dir as created, covering entries
// that appeared before the new directory's watch was in place.
func (w *watcher) addTree(dir, root string, synth bool) {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			return nil
		}
		if p != dir {
			if strings.HasPrefix(d.Name(), tempPrefix) {
				return nil
			}
			if w.m.Match(rel(root, p), d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if synth {
				w.enqueue(Event{Path: p, Op: OpCreate, IsDir: d.IsDir()})
			}
		}
		if d.IsDir() {
			if err := w.watchTreeDir(p, root); err != nil {
				return filepath.SkipDir
			}
		}
		return nil
	})
}

func (w *watcher) watchTreeDir(dir, root string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("vfs: watcher closed")
	}
	if _, ok := w.treeDirs[dir]; ok {
		return nil
	}
	if w.fileDirs[dir] == 0 {
		if err := w.fsw.Add(dir); err != nil {
			w.limitCheckLocked(err)
			return err
		}
	}
	w.treeDirs[dir] = root
	return nil
}

func (w *watcher) unwatchTreeDirLocked(dir string) {
	delete(w.treeDirs, dir)
	if w.fileDirs[dir] == 0 && !w.closed {
		w.fsw.Remove(dir)
	}
}

func isLimitErr(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EMFILE)
}

func (w *watcher) limitCheckLocked(err error) {
	if !isLimitErr(err) || w.limitNotified {
		return
	}
	w.limitNotified = true
	w.bus.Emit(events.TopicNotify, events.Notification{
		ID:    "vfs.watcher.limit",
		Level: "warn",
		Title: "File watcher limit reached",
		Body: "Goi can't watch every folder in this project, so some external changes won't appear " +
			"until you refresh. On Linux, raise fs.inotify.max_user_watches (e.g. " +
			"`sudo sysctl fs.inotify.max_user_watches=524288`) or exclude large folders.",
	})
}

// enqueue merges ev into the current coalescing window.
func (w *watcher) enqueue(ev Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if p, ok := w.pendingIdx[ev.Path]; ok {
		switch {
		case p.Op == OpCreate && ev.Op == OpWrite:
			// still a create
		case p.Op == OpCreate && ev.Op == OpRemove:
			p.dropped = true
			delete(w.pendingIdx, ev.Path)
		default:
			p.Op = ev.Op
			p.IsDir = ev.IsDir
		}
	} else {
		p := &pendingEvent{Event: ev}
		w.pending = append(w.pending, p)
		w.pendingIdx[ev.Path] = p
	}
	if w.timer == nil {
		w.timer = time.AfterFunc(w.window, w.flush)
	}
}

func (w *watcher) flush() {
	w.deliverMu.Lock()
	defer w.deliverMu.Unlock()

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	var batch []Event
	for _, p := range w.pending {
		if !p.dropped {
			batch = append(batch, p.Event)
		}
	}
	w.pending = nil
	w.pendingIdx = map[string]*pendingEvent{}
	w.timer = nil
	subs := make([]func([]Event), 0, len(w.subs))
	for _, fn := range w.subs {
		subs = append(subs, fn)
	}
	w.mu.Unlock()

	if len(batch) == 0 {
		return
	}
	for _, fn := range subs {
		fn(append([]Event(nil), batch...))
	}
	w.bus.Emit(events.TopicFSChanged, Batch{Events: batch})
}

func (w *watcher) fireFileWatches(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	for _, fw := range w.fileWatches[filepath.Dir(name)] {
		if fw.path != name {
			continue
		}
		fw := fw
		if fw.timer != nil {
			fw.timer.Reset(w.debounce)
			continue
		}
		fw.timer = time.AfterFunc(w.debounce, func() {
			w.deliverMu.Lock()
			defer w.deliverMu.Unlock()
			w.mu.Lock()
			closed := w.closed
			w.mu.Unlock()
			if !closed {
				fw.fn()
			}
		})
	}
}
