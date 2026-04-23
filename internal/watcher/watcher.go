package watcher

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ashraf/log-serve/internal/database"
	"github.com/ashraf/log-serve/internal/parser"
)

type Callback func(database.LogRecord)

type Watcher struct {
	serverName   string
	files        []string
	pollInterval time.Duration
	maxLineBytes int
	states       map[string]*fileState
	mu           sync.Mutex
	callback     Callback
}

type fileState struct {
	offset int64
	inode  uint64
}

func New(serverName string, files []string, pollInterval time.Duration, maxLineBytes int, cb Callback) *Watcher {
	return &Watcher{
		serverName:   serverName,
		files:        dedupe(files),
		pollInterval: pollInterval,
		maxLineBytes: maxLineBytes,
		states:       make(map[string]*fileState),
		callback:     cb,
	}
}

func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	w.scan()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scan()
		}
	}
}

func (w *Watcher) scan() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, file := range w.files {
		info, err := os.Stat(file)
		if err != nil || info.IsDir() {
			continue
		}
		state := w.states[file]
		if state == nil {
			state = &fileState{}
			w.states[file] = state
		}
		inode := inodeOf(info)
		if state.inode != 0 && state.inode != inode {
			state.offset = 0
		}
		if info.Size() < state.offset {
			state.offset = 0
		}
		if info.Size() == state.offset {
			state.inode = inode
			continue
		}
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		_, _ = f.Seek(state.offset, io.SeekStart)
		reader := bufio.NewReaderSize(f, 64*1024)
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				state.offset += int64(len(line))
				if len(line) > w.maxLineBytes {
					line = line[:w.maxLineBytes]
				}
				record := parser.Parse(w.serverName, filepath.Base(file), trimLine(line), time.Now())
				w.callback(record)
			}
			if err != nil {
				break
			}
		}
		_ = f.Close()
		state.inode = inode
	}
}

func trimLine(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '\r' {
		s = s[:len(s)-1]
	}
	return s
}

func inodeOf(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
