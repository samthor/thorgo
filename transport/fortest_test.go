package transport

import (
	"testing"
)

func TestPair(t *testing.T) {
	left, right := NewPair(t.Context())

	left.WriteJSON("hello")

	var out string
	right.ReadJSON(&out)

	if out != "hello" {
		t.Errorf("bad send")
	}
}
