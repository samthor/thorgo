package asafe

import (
	"reflect"
	"slices"
	"testing"
)

func TestX(t *testing.T) {
	x := NewSkipQueue(func(a, b int) (is bool) {
		return a < b
	})

	x.Add(1234)
	x.Add(123)

	if !reflect.DeepEqual(x.All(), []int{123, 1234}) {
		t.Errorf("unexpected data")
	}

	x.Add(4000)

	if !reflect.DeepEqual(x.All(), []int{123, 1234, 4000}) {
		t.Errorf("unexpected data")
	}
}

func TestLarge(t *testing.T) {
	x := NewSkipQueue(func(a, b int) (is bool) {
		// a > b, higher values go first (otensibly faster)
		return a > b
	})

	// this goes fast - it's always inserting at head, not tail
	for i := range 1_000_000 {
		x.Add(i)
	}
}

func TestConcurrent(t *testing.T) {
	x := NewSkipQueue(func(a, b int) (is bool) {
		// a > b, higher values go first (otensibly faster)
		return a > b
	})

	tasks := 10
	countPerTask := 4_000 // TODO: roughly doubles every 10k
	doneCh := make(chan bool, tasks)

	for i := range tasks {
		go func() {
			for j := range countPerTask {
				x.Add(i*countPerTask + j)
			}
			doneCh <- true
		}()
	}

	for range tasks {
		<-doneCh
	}

	data := x.All()
	if len(data) != tasks*countPerTask {
		t.Fatalf("wrong count: %v", len(data))
	}

	// assemble expected output (just sort all ints - we "don't care" about prior order)
	var expected []int
	for i := range tasks * countPerTask {
		expected = append(expected, i)
	}
	slices.Sort(data)

	if !reflect.DeepEqual(data, expected) {
		t.Errorf("bad data")
	}
}
