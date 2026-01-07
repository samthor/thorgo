package phttp

import (
	"testing"
)

func TestSelfSignedCerts(t *testing.T) {
	config := buildSelfSignedTLSConfig()
	if config == nil {
		t.Errorf("couldn't build config")
	}
}
