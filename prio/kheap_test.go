package prio

import (
	"reflect"
	"testing"
)

type ComparableInt int

func (c ComparableInt) Less(other ComparableInt) (is bool) {
	return c < other
}

func TestKHeap(t *testing.T) {
	kq := NewHeap[string, ComparableInt]()

	kq.Add("hello", 1)
	kq.Add("hello", 2)
	kq.Add("there", 0)

	values, _ := kq.All()
	if !reflect.DeepEqual(values, []string{"there", "hello"}) {
		t.Errorf("bad order")
	}

	kq.Add("there", 100)
	values, _ = kq.All()
	if !reflect.DeepEqual(values, []string{"hello", "there"}) {
		t.Errorf("bad order")
	}
}

func TestKHeap_PopFront(t *testing.T) {
	kq := NewHeap[string, ComparableInt]()
	kq.Add("mid", 10)
	kq.Add("low", 5)
	kq.Add("high", 20)

	// PopFront should always give the minimum priority
	k, p := kq.Next()
	if k != "low" || p != 5 {
		t.Errorf("expected low/5, got %v/%v", k, p)
	}

	k, p = kq.Next()
	if k != "mid" || p != 10 {
		t.Errorf("expected mid/10, got %v/%v", k, p)
	}

	k, p = kq.Next()
	if k != "high" || p != 20 {
		t.Errorf("expected high/20, got %v/%v", k, p)
	}
}

func TestKHeap_Update(t *testing.T) {
	kq := NewHeap[string, ComparableInt]()

	if !kq.Add("a", 10) {
		t.Error("expected anew=true for first add")
	}
	if kq.Add("a", 10) {
		t.Error("expected anew=false for same priority update")
	}
	if kq.Add("a", 20) {
		t.Error("expected anew=false for different priority update")
	}

	_, p := kq.Next()
	if p != 20 {
		t.Errorf("expected priority 20, got %v", p)
	}
}

func TestKHeap_Empty(t *testing.T) {
	kq := NewHeap[string, ComparableInt]()

	k, p := kq.Next()
	if k != "" || p != 0 {
		t.Error("PopFront on empty should return zero values")
	}

	k, p = kq.Next()
	if k != "" || p != 0 {
		t.Error("PopBack on empty should return zero values")
	}

	if kq.Len() != 0 {
		t.Error("Len should be 0")
	}
}
