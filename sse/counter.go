package sse

import (
	"io"
	"net/http"
)

type writeCounter struct {
	w     io.Writer
	count int64
}

func (wc *writeCounter) Write(b []byte) (int, error) {
	count, err := wc.w.Write(b)
	wc.count += int64(count)
	return count, err
}

func (wc *writeCounter) Flush() {
	if f, ok := wc.w.(http.Flusher); ok {
		f.Flush()
	}
}
