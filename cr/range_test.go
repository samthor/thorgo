package cr

import (
	"reflect"
	"testing"

	"github.com/samthor/thorgo/rope"
)

func prepareSample() (rr rope.Rope[string, string], r CrRange[string]) {
	rr = rope.New[string, string]()
	r = NewRange(rr)

	rr.InsertIdAfter("", "a", 5, "hello")
	rr.InsertIdAfter("a", "b", 5, "there")
	rr.InsertIdAfter("b", "c", 4, ", jim")
	rr.InsertIdAfter("c", "d", 2, "!!")
	rr.InsertIdAfter("d", "e", 12, ", what's up")
	rr.InsertIdAfter("e", "f", 6, " noobs")

	return
}

func TestRange(t *testing.T) {
	rr, r := prepareSample()

	if newlyIncluded, delta, ok := r.Mark("d", "b"); !ok ||
		delta != 6 ||
		!reflect.DeepEqual(newlyIncluded, []string{"b", "d"}) {
		t.Errorf("can't mark sane start: delta=%v newlyIncluded=%+v", delta, newlyIncluded)
	}
	r.Mark("b", "a") // will be swapped

	if r.ExtentCount() != 1 {
		t.Errorf("expected single extent, was: %v", r.ExtentCount())
	}
	if r.Delta() != 11 {
		t.Errorf("expected 11 delta, was: %v", r.Delta())
	}

	r.Mark("e", "f")
	if r.ExtentCount() != 2 {
		t.Errorf("expected double extent, was: %v", r.ExtentCount())
	}

	impl := r.(*rangeOver[string])
	if !reflect.DeepEqual(impl.debugState(), []string{"a", "d", "e", "f"}) {
		t.Errorf("unexpected state: %+v", impl.debugState())
	}

	if newlyIncluded, delta, _ := r.Mark("b", "e"); delta != 12 ||
		!reflect.DeepEqual(newlyIncluded, []string{"d", "e"}) {
		t.Errorf("can't merge: delta=%v newlyIncluded=%+v", delta, newlyIncluded)
	}
	if !reflect.DeepEqual(impl.debugState(), []string{"a", "f"}) {
		t.Errorf("unexpected state: %+v", impl.debugState())
	}

	if actual := impl.debugWithin("b"); !reflect.DeepEqual(actual, []rangeNode[string]{
		{id: "a", delta: +1},
		{id: "b", delta: +1},
		{id: "d", delta: -1},
		{id: "f", delta: -1},
	}) {
		t.Errorf("unexpected within: %+v", actual)
	}

	r.Mark("b", "e")
	if actual := impl.debugWithin("a"); !reflect.DeepEqual(actual, []rangeNode[string]{
		{id: "a", delta: +1},
		{id: "b", delta: +2},
		{id: "d", delta: -1},
		{id: "e", delta: -1},
		{id: "f", delta: -1},
	}) {
		t.Errorf("unexpected within: %+v", actual)
	}

	r.Mark("c", "d")
	if actual := impl.debugWithin("f"); !reflect.DeepEqual(actual, []rangeNode[string]{
		{id: "a", delta: +1},
		{id: "b", delta: +2},
		{id: "c", delta: +1},
		{id: "d", delta: -2},
		{id: "e", delta: -1},
		{id: "f", delta: -1},
	}) {
		t.Errorf("unexpected within: %+v", actual)
	}

	rr.InsertIdAfter("d", "d1", 100, "")
	if !r.Grow("d", 100) {
		t.Errorf("should have grown by 1009")
	}
	if r.Delta() != 129 {
		t.Errorf("got invalid delta, wanted 129 got %v", r.Delta())
	}
}

func TestMultiMerge(t *testing.T) {
	rr, r := prepareSample()

	r.Mark("a", "b")
	r.Mark("c", "d")
	r.Mark("e", "f")

	beforeDelta := r.Delta()

	if newlyIncluded, delta, _ := r.Mark("", "f"); delta != 21 ||
		!reflect.DeepEqual(newlyIncluded, []string{"", "a", "b", "c", "d", "e"}) {
		t.Errorf("can't merge: delta=%v newlyIncluded=%+v", delta, newlyIncluded)
	}

	if beforeDelta+21 != rr.Len() {
		t.Errorf("invalid delta for total delete addition")
	}

	if r.Delta() != rr.Len() {
		t.Errorf("invalid delta for total delete: delta=%v len=%v (bd=%v)", r.Delta(), rr.Len(), beforeDelta)
	}
}

func TestRelease(t *testing.T) {
	_, r := prepareSample()

	r.Mark("b", "f")
	if newlyReleased, delta, ok := r.Release("e", "f"); !ok ||
		delta != -6 ||
		!reflect.DeepEqual(newlyReleased, []string{"e", "f"}) {
		t.Errorf("bad release: delta=%v newlyReleased=%v", delta, newlyReleased)
	}

	// same output because [b,e] is still blocked
	r.Mark("b", "f")
	if newlyReleased, delta, ok := r.Release("b", "f"); !ok ||
		delta != -6 ||
		!reflect.DeepEqual(newlyReleased, []string{"e", "f"}) {
		t.Errorf("bad release: delta=%v newlyReleased=%v", delta, newlyReleased)
	}

	if newlyReleased, delta, ok := r.Release("c", "b"); !ok ||
		delta != -4 ||
		!reflect.DeepEqual(newlyReleased, []string{"b", "c"}) {
		t.Errorf("bad release: delta=%v newlyReleased=%v", delta, newlyReleased)
	}

	if newlyReleased, delta, ok := r.Release("c", "e"); !ok ||
		delta != -14 ||
		!reflect.DeepEqual(newlyReleased, []string{"c", "e"}) {
		t.Errorf("bad release: delta=%v newlyReleased=%v", delta, newlyReleased)
	}

	if _, _, ok := r.Release("c", "e"); ok {
		t.Errorf("should not release")
	}
}

func TestDeltaFor(t *testing.T) {
	_, r := prepareSample()

	if r.DeltaFor("e") != 0 {
		t.Errorf("should be zero delta to start")
	}

	r.Mark("d", "f")
	if r.DeltaFor("e") != 4 {
		t.Errorf("should be...")
	}

}
