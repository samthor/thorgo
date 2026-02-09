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

func TestPull(t *testing.T) {
	q := New[int]()
	p := q.Pull(t.Context())

	q.Push(123)

	next, _ := p(0)
	if !reflect.DeepEqual(next, []int{123}) {
		t.Errorf("bad pull")
	}

	time.AfterFunc(time.Millisecond*10, func() { q.Push(42) })
	next, _ = p(time.Second)
	if !reflect.DeepEqual(next, []int{42}) {
		t.Errorf("bad pull, got: %+v", next)
	}

}

func TestWait(t *testing.T) {
	q := New[int]()
	l := q.Join(context.Background())

	// Test Wait when data is already there
	q.Push(123)
	ch := l.Wait()
	v, ok := <-ch
	if !ok || v != 123 {
		t.Errorf("expected 123, got %v (ok=%v)", v, ok)
	}

	// Test that Wait does not consume the value
	v, ok = l.Next()
	if !ok || v != 123 {
		t.Errorf("Wait should not consume, but Next got %v (ok=%v)", v, ok)
	}

	// Test Wait when data comes later
	waitChResult := make(chan (<-chan int), 1)
	go func() {
		waitChResult <- l.Wait()
	}()

	time.Sleep(time.Millisecond * 10)
	q.Push(456)

	select {
	case waitCh := <-waitChResult:
		v, ok := <-waitCh
		if !ok || v != 456 {
			t.Errorf("expected 456, got %v (ok=%v)", v, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Wait() to return or provide value")
	}

	// Again, should not consume
	v, ok = l.Next()
	if !ok || v != 456 {
		t.Errorf("Wait should not consume (2), but Next got %v (ok=%v)", v, ok)
	}

	// Test Wait with context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	l2 := q.Join(ctx)
	ch3 := l2.Wait()
	cancel()
	select {
	case v, ok := <-ch3:
		if ok {
			t.Errorf("expected closed channel, got %v", v)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Wait() to close on cancellation")
	}
}
