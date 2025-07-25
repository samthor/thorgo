package call

import (
	"context"
	"errors"
	"net/http"

	"github.com/coder/websocket"
	"github.com/samthor/thorgo/transport"
)

func (ch *Handler[Init]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	options := &websocket.AcceptOptions{InsecureSkipVerify: ch.SkipOriginVerify}
	sock, err := websocket.Accept(w, r, options)
	if err != nil {
		// log.Printf("got err setting up websocket %s: %v", r.URL.Path, err)
		http.Error(w, "could not set up websocket", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	if ch.EventStart != nil {
		go ch.EventStart(ctx, r)
	}

	err = ch.runSocket(ctx, r, sock)
	cancel(err)

	var closeError websocket.CloseError
	var transportError transport.TransportError

	if errors.As(err, &transportError) {
		// log.Printf("shutdown socket due to known reason: %+v", transportError)

		closeError.Code = 3000 + websocket.StatusCode(transportError.Code)
		closeError.Reason = transportError.Reason

		sock.Close(closeError.Code, closeError.Reason)
	} else if errors.As(err, &closeError) {
		// log.Printf("shutdown socket due to known reason: %+v", closeError)
		sock.Close(closeError.Code, closeError.Reason)
	} else if err != nil && err != context.Canceled {
		// log.Printf("shutdown socket due to error: %v", err)
		sock.Close(websocket.StatusInternalError, "")
	} else {
		sock.Close(websocket.StatusNormalClosure, "")
	}
}
