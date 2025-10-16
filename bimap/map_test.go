package bimap

import (
	"testing"
)

func TestMap(t *testing.T) {

	var m Map[int, string]

	if !m.Put(1, "abc") {
		t.Errorf("bad")
	}

	if m.Invert().Put("abc", 1) {
		t.Errorf("bad")
	}

	if !m.Invert().DeleteFar(1) {
		t.Errorf("bad")
	}

	if m.Len() != 0 {
		t.Error("bad")
	}

}
