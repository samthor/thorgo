package iter

import (
	"iter"
)

// Seq2Error returns an iter.Seq2 that simply yields a fixed error as its second argument, once.
// It is valid for err to be nil.
func Seq2Error[X any](err error) iter.Seq2[X, error] {
	return func(yield func(X, error) bool) {
		var x X
		yield(x, err)
	}
}

// Seq2Done returns an iter.Seq2 with no values in it.
func Seq2Done[X any, Y any]() iter.Seq2[X, Y] {
	return func(yield func(X, Y) bool) {}
}
