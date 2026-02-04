package main

import (
	"log"
	"net/http"

	"github.com/samthor/thorgo/transport"
)

type Foo struct {
	Hello string `json:"hello"`
}

func main() {

	handler := func(tr transport.Transport) (err error) {
		log.Printf("stuff got conn")

		id := -100
		err = tr.WriteJSON(&transport.ControlPacket[Foo]{
			C: &id,
			P: Foo{Hello: "there"},
		})
		log.Printf("err writing control? %v", err)

		for err == nil {
			var v any
			err = tr.ReadJSON(&v)
			log.Printf("conn got err=%v msg=%+v", err, v)
		}

		return nil
	}

	opts := transport.SocketOpts{}
	h := transport.NewWebSocketHandler(opts, handler)

	http.Handle("/sock", h)
	http.ListenAndServe(":8080", nil)
}
