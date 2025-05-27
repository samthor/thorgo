package cr

import (
	"testing"

	"github.com/samthor/thorgo/rope"
)

func TestRange(t *testing.T) {

	rr := rope.New[string, string]()

	r := NewRange(rr)
	if r.Mark("abc", "def") {
		t.Errorf("cannot insert unknown IDs")
	}

	rr.InsertIdAfter("", "a", 5, "hello")
	rr.InsertIdAfter("a", "b", 5, "there")
	rr.InsertIdAfter("b", "c", 4, ", jim")
	rr.InsertIdAfter("c", "d", 2, "!!")
	rr.InsertIdAfter("d", "e", 12, ", what's up")

	if !r.Mark("b", "d") {
		t.Errorf("can't mark sane start")
	}
	r.Mark("a", "b") // will be swapped
}
