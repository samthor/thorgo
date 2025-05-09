package rope

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

func BenchmarkRope(b *testing.B) {
	for b.Loop() {
		ids := []Id{0}
		r := New[string]()

		for j := 0; j < 500_000; j++ {
			choice := rand.IntN(len(ids))
			afterId := ids[choice]
			newId := r.InsertAfter(afterId, "", rand.IntN(16))
			r.Find(newId)
		}
	}
}

func TestRope(t *testing.T) {
	// run N times to confirm rope behavior
	for i := 0; i < 50; i++ {
		if t.Failed() {
			return
		}

		r := New[string]()

		// insert "hello" and check
		helloId := r.InsertAfter(0, "hello", 5)

		if r.Len() != 5 {
			t.Errorf("expected len=5, was=%v", r.Len())
		}
		if helloId == 0 {
			t.Errorf("expected +ve Id, was=%v", helloId)
		}
		helloAt := r.Find(helloId)
		if helloAt != 0 {
			t.Errorf("expected helloAt=0, was=%v", helloAt)
		}

		// insert " there"
		thereId := r.InsertAfter(helloId, " there", 6)
		if r.Len() != 11 {
			t.Errorf("expected len=11, was=%v", r.Len())
		}

		thereLookup := r.Info(thereId)
		thereAt := r.Find(thereId)

		if thereAt != 5 {
			t.Errorf("expected thereAt=5, was=%v", thereAt)
		}
		if !reflect.DeepEqual(thereLookup, Info[string]{
			Id:     thereId,
			Next:   0,
			Prev:   helloId,
			Length: 6,
			Data:   " there",
		}) {
			t.Errorf("bad lookup=%+v", thereLookup)
		}

		// position
		if id, offset := r.ByPosition(5, false); id != helloId || offset != 5 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, helloId, offset)
		}
		if id, offset := r.ByPosition(5, true); id != thereId || offset != 0 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, thereId, offset)
		}

		// compare
		var cmp int
		var ok bool
		cmp, ok = r.Compare(helloId, thereId)
		if !ok || cmp >= 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}
		cmp, ok = r.Compare(thereId, helloId)
		if !ok || cmp <= 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}
		cmp, ok = r.Compare(thereId, thereId)
		if !ok || cmp != 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}
		cmp, ok = r.Compare(thereId, Id(-1))
		if ok || cmp != 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}

		// delete first
		r.DeleteTo(0, helloId)
		if r.Len() != 6 {
			t.Errorf("didn't reduce by hello size")
		}
		if thereAt = r.Find(thereId); thereAt != 0 {
			t.Errorf("wrong there")
		}

	}

}
