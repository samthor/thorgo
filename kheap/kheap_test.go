package kheap

import (
	"reflect"
	"testing"
)

type ComparableInt int

func (c ComparableInt) Less(other ComparableInt) (is bool) {
	return c < other
}

func TestKHeap(t *testing.T) {
	kq := New[string, ComparableInt]()

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

	kq.PopBack()

	values, _ = kq.All()
	if !reflect.DeepEqual(values, []string{"hello"}) {
		t.Errorf("bad order")
	}
}
