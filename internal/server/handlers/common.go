package handlers

import (
	"bufio"
	"io"
	"net/http"

	"finsd/internal/monitor"
)

var PackageWatcher *monitor.PackageWatcher

type FlushableMultiWriter struct {
	io.Writer
	flusher http.Flusher
}

func NewFlushableMultiWriter(w io.Writer, flusher http.Flusher) *FlushableMultiWriter {
	return &FlushableMultiWriter{
		Writer:  w,
		flusher: flusher,
	}
}

func (w *FlushableMultiWriter) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return
}

func (w *FlushableMultiWriter) Flush() {
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

func StreamCommandOutput(reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		writer.Write(line)
		writer.Write([]byte("\n"))

		if f, ok := writer.(interface{ Flush() }); ok {
			f.Flush()
		}
	}

	return scanner.Err()
}
