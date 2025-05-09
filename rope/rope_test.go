package rope

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

// :. about 60% of JS

//  5000k => 7940848917 ns/op  (7940.85ms/run)
//  1000k =>  850112250 ns/op   (850.11ms/run)
//   500k =>  284802573 ns/op   (284.80ms/run)
//   100k =>   36776703 ns/op    (36.78ms/run)

const (
	benchOps     = 100_000
	deleteOddsOf = 20
)

func BenchmarkRope(b *testing.B) {
	ops := benchOps * (deleteOddsOf - 1) / deleteOddsOf

	for b.Loop() {
		ids := make([]Id, 0, ops)
		ids = append(ids, RootId)
		r := New[struct{}]()

		for j := 0; j < benchOps; j++ {

			if len(ids) <= 2 || rand.IntN(deleteOddsOf) != 0 {
				// insert case
				choice := rand.IntN(len(ids))
				afterId := ids[choice]
				newId := r.InsertAfter(afterId, rand.IntN(16), struct{}{})
				ids = append(ids, newId)

			} else {
				// delete case
				choice := 1 + rand.IntN(len(ids)-2)

				deleteId := ids[choice]
				last := ids[len(ids)-1]
				ids = ids[:len(ids)-1]
				ids[choice] = last

				info := r.Info(deleteId)
				r.DeleteTo(info.Prev, deleteId)
			}
		}
	}
}

func BenchmarkCompare(b *testing.B) {
	r := New[struct{}]()
	ids := []Id{RootId}

	for range 100_000 {
		choice := rand.IntN(len(ids))
		afterId := ids[choice]
		newId := r.InsertAfter(afterId, rand.IntN(16), struct{}{})
		ids = append(ids, newId)
	}

	// before: ~580ns/op

	for b.Loop() {
		a := ids[rand.IntN(len(ids))]
		b := ids[rand.IntN(len(ids))]

		r.Before(a, b)
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
		helloId := r.InsertAfter(RootId, 5, "hello")

		if r.Count() != 1 {
			t.Errorf("expected count=1")
		}
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
		thereId := r.InsertAfter(helloId, 6, " there")
		if r.Len() != 11 {
			t.Errorf("expected len=11, was=%v", r.Len())
		}
		if r.Count() != 2 {
			t.Errorf("expected count=2")
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
		if id, offset := r.ByPosition(0, false); id != RootId || offset != 0 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, RootId, offset)
		}
		if id, offset := r.ByPosition(0, true); id != helloId || offset != 0 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, helloId, offset)
		}

		// compare
		var cmp int
		var ok bool
		cmp, ok = r.Compare(helloId, thereId)
		if !ok || cmp >= 0 {
			t.Errorf("bad cmp for ids (should be -1, hello before there): %v", cmp)
		}
		cmp, ok = r.Compare(thereId, helloId)
		if !ok || cmp <= 0 {
			t.Errorf("bad cmp for ids (should be +1, there not before hello): %v", cmp)
		}
		cmp, ok = r.Compare(thereId, thereId)
		if !ok || cmp != 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}
		cmp, ok = r.Compare(thereId, Id(-1))
		if ok || cmp != 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}

		var out []Id
		for id := range r.Iter(RootId) {
			out = append(out, id)
		}
		if !reflect.DeepEqual(out, []Id{helloId, thereId}) {
			t.Errorf("bad read")
		}

		// delete first
		r.DeleteTo(0, helloId)
		if r.Len() != 6 {
			t.Errorf("didn't reduce by hello size")
		}
		if thereAt = r.Find(thereId); thereAt != 0 {
			t.Errorf("wrong there")
		}
		if r.Count() != 1 {
			t.Errorf("expected count=1")
		}

	}

}
