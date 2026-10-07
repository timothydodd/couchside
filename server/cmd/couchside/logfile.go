package main

import (
	"os"
	"path/filepath"
	"sync"
)

// logFileMax is when the log file is rolled over to <name>.1, keeping one
// old file, so a long-running service's log can't fill the disk.
const logFileMax = 10 << 20

// logFile is an append-only log file that rolls over at logFileMax.
type logFile struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
}

func openLogFile(path string) (*logFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	l := &logFile{path: path}
	return l, l.open()
}

func (l *logFile) open() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.size = f, st.Size()
	return nil
}

func (l *logFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size+int64(len(p)) > logFileMax && l.size > 0 {
		l.f.Close()
		_ = os.Rename(l.path, l.path+".1")
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}
