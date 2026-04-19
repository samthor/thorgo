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

func TestConcurrent(t *testing.T) {
	x := NewSkipQueue(func(a, b int) (is bool) {
		return a < b
	})

	count := 10
	innerCount := 1
	doneCh := make(chan bool, count)

	for i := 0; i < count; i++ {
		go func() {
			for range innerCount {
				x.Add(i)
			}
			doneCh <- true
		}()
	}

	t.Logf("waiting for %d", count)
	for range count {
		<-doneCh
	}

	data := x.All()
	if len(data) != count*innerCount {
		t.Fatalf("wrong count: %v", len(data))
	}

	var expected []int
	for i := range count {
		for j := 0; j < innerCount; j++ {
			expected = append(expected, i)
		}
	}
	slices.Sort(data)

	if !reflect.DeepEqual(data, expected) {
		t.Errorf("bad data")
	}
}
