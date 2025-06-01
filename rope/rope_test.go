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
	internalNextId = 0
)

func nextId() int {
	internalNextId++
	return internalNextId
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
				afterId := ids[choice]

				newId := nextId()
				if !r.InsertIdAfter(afterId, newId, rand.IntN(16), struct{}{}) {
					b.Errorf("couldn't insert")
				}
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
	r := New[int, struct{}]()
	ids := []int{0}

	for range 100_000 {
		choice := rand.IntN(len(ids))
		afterId := ids[choice]

		newId := nextId()
		if !r.InsertIdAfter(afterId, newId, rand.IntN(16), struct{}{}) {
			b.Errorf("couldn't insert")
		}
		ids = append(ids, newId)
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
		helloId := nextId()
		r.InsertIdAfter(0, helloId, 5, "hello")

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
		if helloAt != 5 {
			t.Errorf("expected helloAt=5, was=%v", helloAt)
		}

		// insert " there"
		thereId := nextId()
		r.InsertIdAfter(helloId, thereId, 6, " there")
		if r.Len() != 11 {
			t.Errorf("expected len=11, was=%v", r.Len())
		}
		if r.Count() != 2 {
			t.Errorf("expected count=2")
		}

		thereLookup := r.Info(thereId)
		thereAt := r.Find(thereId)

		if thereAt != 11 {
			t.Errorf("expected thereAt=11, was=%v", thereAt)
		}
		if !reflect.DeepEqual(thereLookup, Info[int, string]{
			Id:      thereId,
			Next:    0,
			Prev:    helloId,
			DataLen: DataLen[string]{Data: " there", Len: 6},
		}) {
			t.Errorf("bad lookup=%+v", thereLookup)
		}

		// position
		if id, offset := r.ByPosition(5, false); id != helloId || offset != 0 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, helloId, offset)
		}
		if id, offset := r.ByPosition(5, true); id != thereId || offset != 6 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, thereId, offset)
		}
		if id, offset := r.ByPosition(0, false); id != 0 || offset != 0 {
			t.Errorf("bad byPosition: id=%d (wanted=%d), offset=%d", id, 0, offset)
		}
		if id, offset := r.ByPosition(0, true); id != helloId || offset != 5 {
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
		cmp, ok = r.Compare(thereId, -1)
		if ok || cmp != 0 {
			t.Errorf("bad cmp for ids: %v", cmp)
		}

		var out []int
		for id := range r.Iter(0) {
			out = append(out, id)
		}
		if !reflect.DeepEqual(out, []int{helloId, thereId}) {
			t.Errorf("bad read")
		}

		// delete first
		count := r.DeleteTo(0, helloId)
		if count != 1 {
			t.Errorf("expected deleted one, was: %v", count)
		}
		if r.Len() != 6 {
			t.Errorf("didn't reduce by hello size: wanted=%d, got=%d", 6, r.Len())
		}
		if thereAt = r.Find(thereId); thereAt != 6 {
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
			if !r.InsertIdAfter(parent, nextId(), length, s) {
				t.Errorf("couldn't insert")
			}
		}
	}
}

func TestIter(t *testing.T) {
	r := New[int, string]()

	r.InsertIdAfter(0, 1, 5, "hello")
	if r.LastId() != 1 {
		t.Errorf("should have first lastId")
	}

	r.InsertIdAfter(1, 2, 6, " there")
	if r.LastId() != 2 {
		t.Errorf("should have second lastId")
	}

	r.InsertIdAfter(2, 3, 4, " bob")
	if r.LastId() != 3 {
		t.Errorf("should have third lastId")
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

	if r.LastId() != 2 {
		t.Errorf("should have second lastId, was=%v", r.LastId())
	}
	r.DeleteTo(0, 2)

	if r.Count() != 0 {
		t.Errorf("should hve no entries")
	}

	id, value, ok = next()
	if ok {
		t.Errorf("should not get more values: last deleted: was=%v %v", id, value)
	}

	if r.LastId() != 0 {
		t.Errorf("should have zero lastId, was=%v", r.LastId())
	}
}
