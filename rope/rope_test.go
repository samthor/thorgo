package rope

import (
	"iter"
	"math/rand/v2"
	"reflect"
	"testing"
)

// :. about 2x JS speed

//  5000k => 6807085083 ns/op  (6807.09ms/run)
//  1000k =>  678856229 ns/op   (678.86ms/run)
//   500k =>  245334542 ns/op   (245.33ms/run)
//   100k =>   28436903 ns/op    (28.44ms/run)

const (
	benchOps     = 100_000
	deleteOddsOf = 20
)

var (
	internalNextID = 0
)

func nextID() int {
	internalNextID++
	return internalNextID
}

func BenchmarkRope(b *testing.B) {
	ops := benchOps * (deleteOddsOf - 1) / deleteOddsOf
	ids := make([]int, 0, ops)

	for b.Loop() {
		ids = ids[:0]
		ids = append(ids, 0)
		r := New[int, struct{}]()

		for j := 0; j < benchOps; j++ {

			if len(ids) <= 2 || rand.IntN(deleteOddsOf) != 0 {
				// insert case
				choice := rand.IntN(len(ids))
				afterID := ids[choice]

				newID := nextID()
				if !r.InsertIDAfter(afterID, newID, rand.IntN(16), struct{}{}) {
					b.Errorf("couldn't insert")
				}
				ids = append(ids, newID)

			} else {
				// delete case
				choice := 1 + rand.IntN(len(ids)-2)

				deleteID := ids[choice]
				last := ids[len(ids)-1]
				ids = ids[:len(ids)-1]
				ids[choice] = last

				info := r.Info(deleteID)
				r.DeleteTo(info.Prev, deleteID)
			}
		}
	}
}

func BenchmarkCompare(b *testing.B) {
	r := New[int, struct{}]()
	ids := []int{0}

	for range 100_000 {
		choice := rand.IntN(len(ids))
		afterID := ids[choice]

		newID := nextID()
		if !r.InsertIDAfter(afterID, newID, rand.IntN(16), struct{}{}) {
			b.Errorf("couldn't insert")
		}
		ids = append(ids, newID)
	}

	// before: ~580ns/op

	for b.Loop() {
		a := ids[rand.IntN(len(ids))]
		b := ids[rand.IntN(len(ids))]

		r.Less(a, b)
	}
}

func TestRope(t *testing.T) {
	// run N times to confirm rope behavior
	for i := 0; i < 50; i++ {
		if t.Failed() {
			return
		}

		r := New[int, string]()

		// insert "hello" and check
		helloID := nextID()
		r.InsertIDAfter(0, helloID, 5, "hello")

		if r.Count() != 1 {
			t.Errorf("expected count=1")
		}
		if r.Len() != 5 {
			t.Errorf("expected len=5, was=%v", r.Len())
		}
		if helloID == 0 {
			t.Errorf("expected +ve ID, was=%v", helloID)
		}
		helloAt := r.Find(helloID)
		if helloAt != 5 {
			t.Errorf("expected helloAt=5, was=%v", helloAt)
		}

		// insert " there"
		thereID := nextID()
		r.InsertIDAfter(helloID, thereID, 6, " there")
		if r.Len() != 11 {
			t.Errorf("expected len=11, was=%v", r.Len())
		}
		if r.Count() != 2 {
			t.Errorf("expected count=2")
		}

		thereLookup := r.Info(thereID)
		thereAt := r.Find(thereID)

		if thereAt != 11 {
			t.Errorf("expected thereAt=11, was=%v", thereAt)
		}
		if !reflect.DeepEqual(thereLookup, Info[int, string]{
			ID:      thereID,
			Next:    0,
			Prev:    helloID,
			DataLen: DataLen[string]{Data: " there", Len: 6},
		}) {
			t.Errorf("bad lookup=%+v", thereLookup)
		}

		// position
		if id, offset := r.ByPosition(5, false); id != helloID || offset != 0 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, helloID, offset)
		}
		if id, offset := r.ByPosition(5, true); id != thereID || offset != 6 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, thereID, offset)
		}
		if id, offset := r.ByPosition(0, false); id != 0 || offset != 0 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, 0, offset)
		}
		if id, offset := r.ByPosition(0, true); id != helloID || offset != 5 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, helloID, offset)
		}

		// compare
		var cmp int
		var ok bool
		cmp, ok = r.Compare(helloID, thereID)
		if !ok || cmp >= 0 {
			t.Errorf("bad cmp for ids (should be -1, hello before there): %v", cmp)
		}
		cmp, ok = r.Compare(thereID, helloID)
		if !ok || cmp <= 0 {
			t.Errorf("bad cmp for ids (should be +1, there not before hello): %v", cmp)
		}
		cmp, ok = r.Compare(thereID, thereID)
		if !ok || cmp != 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}
		cmp, ok = r.Compare(thereID, -1)
		if ok || cmp != 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}

		var out []int
		for id := range r.Iter(0) {
			out = append(out, id)
		}
		if !reflect.DeepEqual(out, []int{helloID, thereID}) {
			t.Errorf("bad read")
		}

		// delete first
		count := r.DeleteTo(0, helloID)
		if count != 1 {
			t.Errorf("expected deleted one, was: %v", count)
		}
		if r.Len() != 6 {
			t.Errorf("didn't reduce by hello size: wanted=%d, got=%d", 6, r.Len())
		}
		if thereAt = r.Find(thereID); thereAt != 6 {
			t.Errorf("wrong there: %d", thereAt)
		}
		if r.Count() != 1 {
			t.Errorf("expected count=1")
		}

	}
}

func TestRandomRope(t *testing.T) {
	ids := make([]int, 0, 51)

	for i := 0; i < 100; i++ {
		r := New[int, string]()
		ids = ids[:0]
		ids = append(ids, 0)

		for j := 0; j < 50; j++ {

			choice := rand.IntN(len(ids))
			parent := ids[choice]

			length := rand.IntN(4) + 1
			var s string
			for range length {
				s += string(rune('a' + rand.IntN(26)))
			}
			if !r.InsertIDAfter(parent, nextID(), length, s) {
				t.Errorf("couldn't insert")
			}
		}
	}
}

func TestIter(t *testing.T) {
	r := New[int, string]()

	r.InsertIDAfter(0, 1, 5, "hello")
	if r.LastID() != 1 {
		t.Errorf("should have first lastID")
	}

	r.InsertIDAfter(1, 2, 6, " there")
	if r.LastID() != 2 {
		t.Errorf("should have second lastID")
	}

	r.InsertIDAfter(2, 3, 4, " bob")
	if r.LastID() != 3 {
		t.Errorf("should have third lastID")
	}

	// check delete self

	i := r.Iter(0)
	next, stop := iter.Pull2(i)
	defer stop()

	id, value, _ := next()
	if id != 1 || value.Data != "hello" {
		t.Errorf("bad first next")
	}

	// delete item we just returned
	if r.DeleteTo(1, 1) != 0 {
		t.Errorf("should not delete any with same values")
	}
	if r.DeleteTo(0, 1) != 1 {
		t.Errorf("should delete one")
	}

	id, value, _ = next()
	if id != 2 || value.Data != " there" {
		t.Errorf("bad additional next, got: id=%d value=%v", id, value)
	}

	r.DeleteTo(2, 3)
	_, _, ok := next()
	if ok {
		t.Errorf("should not get more values: last deleted")
	}

	// check delete future entry

	i = r.Iter(0)
	next, stop = iter.Pull2(i)
	defer stop()

	if r.Count() != 1 {
		t.Errorf("should hve single entry")
	}

	if r.LastID() != 2 {
		t.Errorf("should have second lastID, was=%v", r.LastID())
	}
	r.DeleteTo(0, 2)

	if r.Count() != 0 {
		t.Errorf("should hve no entries")
	}

	id, value, ok = next()
	if ok {
		t.Errorf("should not get more values: last deleted: was=%v %v", id, value)
	}

	if r.LastID() != 0 {
		t.Errorf("should have zero lastID, was=%v", r.LastID())
	}
}
