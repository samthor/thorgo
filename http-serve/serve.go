package h2

import (
	"net/http"
	"os"
	"strconv"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// ListenAndServe serves HTTP traffic in a sensibly default way.
//
// By default, it serves on the env PORT or port 8080 and supports H2C.
// It can also be configured with self-signed SSL, useful for some providers.
func ListenAndServe(opts *ListenAndServeOpts) error {
	if opts == nil {
		opts = &ListenAndServeOpts{}
	}

	// decide on addr
	addr := opts.Addr
	if addr == "" {
		port, _ := strconv.Atoi(os.Getenv("PORT"))
		if port <= 0 {
			port = 8080
		}

		host := "localhost"
		if opts.ServeAll {
			host = ""
		}

		addr = host + ":" + strconv.Itoa(port)
	}

	// get handler
	handler := opts.Handler
	if handler == nil {
		handler = http.DefaultServeMux
	}

	// h2c
	// TODO: maybe can use Protocols and SetUnencryptedHTTP2
	h2s := &http2.Server{}
	handler = h2c.NewHandler(handler, h2s)

	// actually serve
	s := http.Server{Addr: addr, Handler: handler}
	if opts.FakeSSL {
		s.TLSConfig = buildSelfSignedTLSConfig()
		return s.ListenAndServeTLS("", "")
	}
	return s.ListenAndServe()
}

type ListenAndServeOpts struct {
	// Addr is the address to listen on.
	// If not passed, looks for the PORT env var or defaults to ":8080".
	Addr string

	// ServeAll hosts the server on all addresses (vs localhost) if Addr is unspecified.
	ServeAll bool

	// Handler is the handler to serve.
	// If nil, uses [http.DefaultServeMux].
	Handler http.Handler

	// FakeSSL runs the handler with self-signed TLS.
	// As of July 2025, this is useful for Cloudflare, which allows HTTP/2 over "bad" SSL (rather than h2c).
	FakeSSL bool
}
