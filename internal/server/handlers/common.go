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

// LineFlushWriter 按行刷新的 Writer，用于实时输出命令执行结果
type LineFlushWriter struct {
	target  io.Writer
	flusher interface{ Flush() }
	buffer  []byte
}

func NewLineFlushWriter(w io.Writer) *LineFlushWriter {
	var flusher interface{ Flush() }
	if f, ok := w.(interface{ Flush() }); ok {
		flusher = f
	}
	return &LineFlushWriter{
		target:  w,
		flusher: flusher,
		buffer:  make([]byte, 0, 4096),
	}
}

func (w *LineFlushWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	
	// 逐字节处理，遇到换行符就刷新
	for _, b := range p {
		w.buffer = append(w.buffer, b)
		if b == '\n' {
			// 写入完整的一行
			if _, err := w.target.Write(w.buffer); err != nil {
				return n, err
			}
			// 立即刷新
			if w.flusher != nil {
				w.flusher.Flush()
			}
			// 清空缓冲区
			w.buffer = w.buffer[:0]
		}
	}
	
	// 如果还有未写入的数据（没有换行符结尾），也写入并刷新
	if len(w.buffer) > 0 {
		if _, err := w.target.Write(w.buffer); err != nil {
			return n, err
		}
		if w.flusher != nil {
			w.flusher.Flush()
		}
		w.buffer = w.buffer[:0]
	}
	
	return n, nil
}

func (w *LineFlushWriter) Flush() {
	// 刷新剩余缓冲区
	if len(w.buffer) > 0 {
		w.target.Write(w.buffer)
		w.buffer = w.buffer[:0]
	}
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
