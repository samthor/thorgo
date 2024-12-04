package queue

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestQueue(t *testing.T) {
	q := New[int]()

	go func() {
		obs := q.Join(context.Background())

		var out []int

		out = obs.Batch()
		if !reflect.DeepEqual(out, []int{1, 2, 3}) {
			t.Errorf("expected 1,2,3, was: %+v", out)
		}

		out = obs.Batch()
		if !reflect.DeepEqual(out, []int{4}) {
			t.Errorf("expected 4, was: %+v", out)
		}

		go func() {
			obs2 := q.Join(context.Background())
			out2 := obs2.Batch()
			if !reflect.DeepEqual(out2, []int{5}) {
				t.Errorf("expected 5, was: %+v", out2)
			}
		}()

		out = obs.Batch()
		if !reflect.DeepEqual(out, []int{5}) {
			t.Errorf("expected 5, was: %+v", out)
		}
	}()

	time.Sleep(time.Millisecond * 10)
	q.Push(1, 2, 3)

	time.Sleep(time.Millisecond * 10)
	q.Push(4)

	time.Sleep(time.Millisecond * 10)
	q.Push(5)

	time.Sleep(time.Millisecond * 10)
}

func TestPeek(t *testing.T) {
	q := New[int]()

	l := q.Join(context.Background())

	var value int
	var ok bool

	value, ok = l.Peek()
	if ok {
		t.Errorf("got value when initially peeking: %v", value)
	}

	q.Push(123)
	value, ok = l.Peek()
	if value != 123 || !ok {
		t.Errorf("got unexpected value when peeking: %v", value)
	}

	q.Push(456)
	value, ok = l.Peek() // should be same
	if value != 123 || !ok {
		t.Errorf("got unexpected value when peeking: %v", value)
	}

	value, ok = l.Next()
	if value != 123 || !ok {
		t.Errorf("got unexpected value when fetching 1/2: %v", value)
	}

	value, ok = l.Peek()
	if value != 456 || !ok {
		t.Errorf("got unexpected value when peeking: %v", value)
	}

	value, ok = l.Next()
	if value != 456 || !ok {
		t.Errorf("got unexpected value when fetching 2/2: %v", value)
	}

	value, ok = l.Peek()
	if ok {
		t.Errorf("got value when peeking at end: %v", value)
	}
}
