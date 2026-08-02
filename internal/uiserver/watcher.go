package uiserver

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches the project's collections/ and environments/ directories
// and broadcasts files.changed frames (with a fresh tree etag) over the hub,
// debounced (spec §5.1, §10.6.1.2).
type Watcher struct {
	fsw      *fsnotify.Watcher
	server   *Server
	debounce time.Duration
	mu       sync.Mutex
	pending  map[string]struct{}
	timer    *time.Timer
	closed   chan struct{}
}

// newWatcher starts watching. Failure to construct is non-fatal for the
// server (the tree just won't live-refresh); callers log and continue.
func newWatcher(s *Server) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		fsw:      fsw,
		server:   s,
		debounce: 300 * time.Millisecond,
		pending:  map[string]struct{}{},
		closed:   make(chan struct{}),
	}
	for _, dir := range []string{s.opts.Root, filepath.Join(s.opts.Root, "collections"), filepath.Join(s.opts.Root, "environments")} {
		_ = fsw.Add(dir) // missing dirs are tolerated; created later → root event re-adds
	}
	go w.loop()
	return w, nil
}

func (w *Watcher) loop() {
	for {
		select {
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}
			ext := filepath.Ext(ev.Name)
			if ext != ".yaml" && ext != ".yml" {
				// A created collections/ or environments/ dir starts being watched.
				if ev.Op&fsnotify.Create != 0 {
					base := filepath.Base(ev.Name)
					if base == "collections" || base == "environments" {
						_ = w.fsw.Add(ev.Name)
					}
				}
				continue
			}
			rel, err := filepath.Rel(w.server.opts.Root, ev.Name)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			w.mu.Lock()
			w.pending[rel] = struct{}{}
			if w.timer == nil {
				w.timer = time.AfterFunc(w.debounce, w.flush)
			} else {
				w.timer.Reset(w.debounce)
			}
			w.mu.Unlock()
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
		case <-w.closed:
			return
		}
	}
}

// flush broadcasts the accumulated changed paths with a fresh tree etag.
func (w *Watcher) flush() {
	w.mu.Lock()
	paths := make([]string, 0, len(w.pending))
	for p := range w.pending {
		paths = append(paths, p)
	}
	w.pending = map[string]struct{}{}
	w.timer = nil
	w.mu.Unlock()
	if len(paths) == 0 {
		return
	}
	etag := w.server.treeEtag(w.server.collectionPaths())
	w.server.hub.Broadcast(marshalFrame("files.changed", "", map[string]any{
		"paths":     paths,
		"tree_etag": etag,
	}))
}

// Close stops the watcher.
func (w *Watcher) Close() {
	close(w.closed)
	_ = w.fsw.Close()
}
