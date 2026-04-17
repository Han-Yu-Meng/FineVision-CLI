package handlers

import (
	"bufio"
	"io"
	"net/http"

	"finsd/internal/monitor"
)

// PackageWatcher 全局包监视器
var PackageWatcher *monitor.PackageWatcher

// FlushableMultiWriter 用于流式输出，确保数据立即推送到 HTTP 客户端
type FlushableMultiWriter struct {
	io.Writer
	flusher http.Flusher
}

// NewFlushableMultiWriter 创建一个自动刷新的 MultiWriter
func NewFlushableMultiWriter(w io.Writer, flusher http.Flusher) *FlushableMultiWriter {
	return &FlushableMultiWriter{
		Writer:  w,
		flusher: flusher,
	}
}

func (w *FlushableMultiWriter) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	// 每次写入后立即刷新
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

// StreamCommandOutput 实时流式输出命令执行结果
func StreamCommandOutput(reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		writer.Write(line)
		writer.Write([]byte("\n"))

		// 立即刷新
		if f, ok := writer.(interface{ Flush() }); ok {
			f.Flush()
		}
	}

	return scanner.Err()
}
