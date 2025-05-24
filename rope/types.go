// package rope implements a skip list.

package rope

import (
	"iter"
)

// Info is a holder for info looked up in a Rope.
type Info[Id comparable, T any] struct {
	Id, Next, Prev Id
	DataLen[T]
}

// DataLen is a pair type.
type DataLen[T any] struct {
	Len  int
	Data T
}

// Rope is a skip list.
// It supports zero-length entries.
// It is not goroutine-safe.
// The zero Id is always part of the Rope and has zero length, don't use it to add items.
type Rope[Id comparable, T any] interface {
	// Returns the total sum of the parts of the rope. O(1).
	Len() int

	// Returns the number of parts here. O(1).
	Count() int

	// Finds the position of the given Id.
	// This lookup costs ~O(logn).
	Find(id Id) int

	// Finds info on the given Id.
	// This lookup costs O(1).
	Info(id Id) Info[Id, T]

	// Finds the Id/info at the position in the Rope.
	// This costs ~O(logn).
	// Either stops before or skips after zero-length content based on biasAfter.
	// e.g., with 0/false, this will aways return the zero Id.
	ByPosition(position int, biasAfter bool) (id Id, offset int)

	// Compare the two Id in this Rope.
	// Costs ~O(logn).
	Compare(a, b Id) (cmp int, ok bool)

	// Less determines if the first Id in this Rope before the other. For sorting.
	// Costs ~O(logn).
	Less(a, b Id) bool

	// Iter reads from after the given Id.
	// It is safe to use even if the Rope is modified.
	Iter(afterId Id) iter.Seq2[Id, DataLen[T]]

	// Inserts a new entry after the prior Id.
	// This will panic if the length is negative.
	InsertIdAfter(afterId, id Id, length int, data T) bool

	// DeleteTo deletes after the given Id until the target Id.
	// Pass zero/root for all content after.
	// Costs ~O(logn+m), where m is the number of nodes being deleted.
	DeleteTo(afterId, untilId Id) int
}
