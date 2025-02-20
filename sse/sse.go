// Package sse helps you send Sever-Sent Event streams over HTTP.
package sse

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type hasString interface {
	String() string
}

var (
	ErrBadFormat = errors.New("can't have newline")
)

func writeLines(w io.Writer, left string, b []byte) error {
	for len(b) != 0 {
		newline := bytes.IndexRune(b, '\n')
		if newline == -1 {
			break
		}
		fmt.Fprintf(w, "%s: ", left)
		w.Write(b[0:newline])
		w.Write([]byte{'\n'})
		b = b[newline+1:]
	}
	fmt.Fprintf(w, "%s: ", left)
	w.Write(b)
	w.Write([]byte{'\n'})
	return nil
}

// SetHeaders adds the required SSE headers to the output Headers.
func SetHeaders(h http.Header) {
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
}

// Write writes the given SSE message to the io.Writer.
func Write(w io.Writer, m Message) (int, error) {
	wc := &writeCounter{w: w}
	err := internalWrite(wc, &m)
	return int(wc.count), err
}

func internalWrite(w io.Writer, m *Message) error {
	if strings.ContainsRune(m.Event, '\n') || strings.ContainsRune(m.ID, '\n') {
		return ErrBadFormat
	}

	var err error
	var emitAny bool

	defer func() {
		if !emitAny {
			// add empty comment
			fmt.Fprint(w, ":\n")
		}
		fmt.Fprint(w, "\n")

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}()

	if m.Comment != "" {
		err = writeLines(w, "", []byte(m.Comment))
		if err != nil {
			return err
		}
		emitAny = true
	}

	if m.Event != "" {
		_, err = fmt.Fprintf(w, "event: %s\n", m.Event)
		if err != nil {
			return err
		}
		emitAny = true
	}

	if m.ID != "" {
		_, err = fmt.Fprintf(w, "id: %s\n", m.ID)
		if err != nil {
			return err
		}
		emitAny = true
	}

	if m.Retry > 0 {
		_, err = fmt.Fprintf(w, "retry: %d\n", m.Retry.Milliseconds())
		if err != nil {
			return err
		}
		emitAny = true
	}

	var b []byte

	if m.JSON {
		fmt.Fprint(w, "data: ")
		err := json.NewEncoder(w).Encode(m.Data) // includes newline
		if err != nil {
			return err
		}
		emitAny = true
		return nil
	} else if reader, ok := m.Data.(io.Reader); ok {
		b, err = io.ReadAll(reader)
	} else if hs, ok := m.Data.(hasString); ok {
		b = []byte(hs.String())
	} else if m.Data != nil {
		b = []byte(fmt.Sprintf("%s", m.Data))
	}

	if err != nil {
		return err
	}
	if b != nil {
		emitAny = true
		return writeLines(w, "data", b)
	}
	return nil
}

// Message represents a message that can be sent over Server-Sent Events.
type Message struct {
	Comment string
	Event   string
	ID      string
	Retry   time.Duration
	Data    interface{}
	JSON    bool
}

// WriteTo nominally implements io.WriteTo.
func (m *Message) WriteTo(w io.Writer) (int64, error) {
	if m == nil {
		return 0, nil
	}
	wc := &writeCounter{w: w}
	err := internalWrite(wc, m)
	return wc.count, err
}
