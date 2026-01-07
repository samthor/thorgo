package phttp

import (
	"log"
)

// Run runs ListenAndServe forever, failing when shut down.
// This will not return.
func Run(opts *ListenAndServeOpts) {
	err := ListenAndServe(opts)
	log.Fatalf("shutdown: %v", err)
}
