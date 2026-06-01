package client

import (
	"io"
	"sync"

	"github.com/gookit/cliui/progress"
)

func downloadProgressWriter(getbar func(size int64) io.Writer, size int64) io.Writer {
	if getbar == nil {
		return io.Discard
	}
	bar := getbar(size)
	if bar == nil {
		return io.Discard
	}
	return bar
}

func concurrentProgressWriter(bar io.Writer) (io.Writer, func()) {
	if bar == nil || bar == io.Discard {
		return io.Discard, func() {}
	}
	if p, ok := bar.(*progress.Progress); ok {
		writer := progress.NewConcurrentWriterWithInterval(p, downloadProgressFlushInterval)
		return writer, func() { _ = writer.Close() }
	}
	if closer, ok := bar.(io.Closer); ok {
		return &lockedWriter{writer: bar}, func() { _ = closer.Close() }
	}
	return &lockedWriter{writer: bar}, func() {}
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}
