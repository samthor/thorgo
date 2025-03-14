package wrap

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/coder/websocket"
	"github.com/samthor/thorgo/transport"
)

// HttpFunc is a handler used by Http which allows generating simple result types.
// Return nil to skip the built-in behavior.
type HttpFunc func(http.ResponseWriter, *http.Request) interface{}

// Http returns a http.HandlerFunc that wraps a HttpFunc capable of convenient return types.
func Http(fn HttpFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error

		out := fn(w, r)
		switch x := out.(type) {
		case nil:
			return
		case error:
			err = x
		case []byte:
			w.Write(x)
		case io.Reader:
			if rc, ok := x.(io.ReadCloser); ok {
				defer rc.Close()
			}
			_, err = io.Copy(w, x)
		case string:
			w.Write([]byte(x))
		case int:
			w.WriteHeader(x)
		default:
			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(x)
		}

		if err == nil {
			return
		}
		log.Printf("got err handling %s: err=%v", r.URL.Path, err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// WebSocketTransport returns a http.HandlerFunc that wraps a websocket setup/teardown into a transport.Transport which sends and receives JSON packets.
func WebSocketTransport(fn func(t transport.Transport) error, options *websocket.AcceptOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sock, err := websocket.Accept(w, r, options)
		if err != nil {
			log.Printf("got err setting up websocket %s: %v", r.URL.Path, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		t, cancel := transport.SocketJSON(r.Context(), sock)
		err = fn(t)
		cancel(err)
	}
}
