package sse

import (
	"io"
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
