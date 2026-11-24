package activity

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

var defaultOutput = &lockedOutput{writer: os.Stdout}

type lockedOutput struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}

// ConfigureLogFile writes UTF-8 JSONL directly, independent of shell encoding.
// Call before opening database pools so auth, HTTP and SQL share the same sink.
func ConfigureLogFile(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defaultOutput.mu.Lock()
	previous := defaultOutput.writer
	defaultOutput.writer = io.MultiWriter(file, os.Stdout)
	defaultOutput.mu.Unlock()
	return func() error {
		defaultOutput.mu.Lock()
		defer defaultOutput.mu.Unlock()
		defaultOutput.writer = previous
		return file.Close()
	}, nil
}
