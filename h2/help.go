package h2

import (
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// Handler wraps the given http.Handler such that it can now serve unencrypted h2 traffic.
// This is useful for hosting providers.
func Handler(h http.Handler) http.Handler {
	if h == nil {
		// h2c requires this to be passed
		h = http.DefaultServeMux
	}
	h2s := &http2.Server{}
	return h2c.NewHandler(h, h2s)
}
