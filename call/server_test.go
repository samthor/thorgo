package call

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/samthor/thorgo/transport"
)

const (
	testNoopTimeout = time.Second / 4
)

func setupTestServer(t *testing.T) (server *httptest.Server, conn *websocket.Conn) {
	handler := http.NewServeMux()

	h := &Handler[struct{}]{
		CallHandler: func(ac transport.Transport, init struct{}) error {
			var x struct {
				Test string `json:"test"`
			}
			err := ac.ReadJSON(&x)
			if x.Test != "hello" {
				if !errors.Is(err, io.EOF) && errors.Is(err, websocket.CloseError{}) {
					t.Errorf("did not get 'hello' over socket: %v err=%v", x.Test, err)
				}
			}
			return nil
		},

		CallLimit: &LimitConfig{
			Burst: 2,
			Rate:  0,
		},

		noopTimeout: testNoopTimeout,
	}
	handler.Handle("/s", h)

	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)

	conn, _, err := websocket.Dial(t.Context(), s.URL+"/s", nil)
	if err != nil {
		t.Errorf("couldn't connect to socket")
	}
	t.Cleanup(func() { conn.CloseNow() })

	wsjson.Write(t.Context(), conn, struct {
		Protocol string `json:"p"`
	}{Protocol: "1"})

	var out helloResponseMessage[struct{}]
	wsjson.Read(t.Context(), conn, &out)
	if out.Ok != true {
		t.Error("non-ok")
	}

	return server, conn
}

func TestServer(t *testing.T) {
	_, conn := setupTestServer(t)

	conn.Write(t.Context(), websocket.MessageText, []byte(`:{"c":1}`))

	var m struct {
		Test string `json:"test"`
	}
	m.Test = "hello"
	wsjson.Write(t.Context(), conn, m)

	timeoutCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, b, err := conn.Read(timeoutCtx)
	if !bytes.Equal(b, []byte(`:{"c":1,"stop":""}`)) {
		t.Errorf("should have had stop message, was: %s (err=%v)", string(b), err)
	}

	timeoutCtx, cancel = context.WithTimeout(t.Context(), testNoopTimeout*2)
	defer cancel()

	_, b, err = conn.Read(timeoutCtx)
	if !bytes.Equal(b, []byte(`:{}`)) {
		t.Errorf("should have had noop message, was: %s (err=%v)", string(b), err)
	}
}

func TestLoad(t *testing.T) {
	_, conn := setupTestServer(t)

	for i := range 10 {
		conn.Write(t.Context(), websocket.MessageText, []byte(fmt.Sprintf(`:{"c":%d}`, i+1)))
	}
	time.Sleep(time.Second / 4)

	timeoutCtx, cancel := context.WithTimeout(t.Context(), testNoopTimeout*2)
	defer cancel()
	_, _, err := conn.Read(timeoutCtx)
	if err == nil {
		t.Errorf("expected shutdown due to spam, was: %v", err)
	}

	var ce websocket.CloseError
	if !errors.As(err, &ce) || ce.Code != SocketCodeExcessTraffic {
		t.Errorf("expected CloseError")
	}
}
