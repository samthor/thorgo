package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSock(t *testing.T) {
	msgCh := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/sock", func(w http.ResponseWriter, r *http.Request) {
		sock, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Errorf("err accepting socket: %v", err)
		}

		ctx, cancel := context.WithCancel(r.Context())
		tr := SocketJSON(ctx, sock)
		defer cancel()

		var out struct {
			Message string `json:"message"`
		}
		tr.Read(&out)

		if out.Message != "hi there" {
			t.Errorf("bad message")
		}

		close(msgCh)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	sock, _, err := websocket.Dial(t.Context(), server.URL+"/sock", nil)
	if err != nil {
		t.Errorf("could not create sock: %v", err)
	}

	tr := SocketJSON(context.Background(), sock)
	err = tr.Send(map[string]string{"message": "hi there"})
	if err != nil {
		t.Errorf("could not send test payload: %v", err)
	}

	select {
	case <-msgCh:
	case <-time.After(time.Second * 4):
		t.Errorf("failed after 4s")
	}
}
