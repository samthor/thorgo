// package rope implements a skip list.

package rope

import (
	"iter"
)

var (
	// RootId is the zero ID for all ropes.
	RootId = Id(0)
)

// Id is the opaque ID given when you add to a Rope.
type Id int

// Info is a holder for info looked up in a Rope.
type Info[T any] struct {
	Id, Next, Prev Id
	Length         int
	Data           T
}

// Rope is a skip list.
// It supports zero-length entries.
// It is not goroutine-safe.
type Rope[T any] interface {
	// Returns the total sum of the parts of the rope. O(1).
	Len() int

	// Returns the number of parts here. O(1).
	Count() int

	// Finds the position of the given Id.
	// This lookup costs ~O(logn).
	Find(id Id) int

	// Finds info on the given Id.
	// This lookup costs O(1).
	Info(id Id) Info[T]

	// Finds the Id/info at the position in the Rope.
	// This costs ~O(logn).
	// Either stops before or skips after zero-length content based on biasAfter.
	// e.g., with 0/false, this will aways return Id=0 (the root).
	ByPosition(position int, biasAfter bool) (id Id, offset int)

	// Compare the two Id in this Rope.
	// Costs ~O(logn).
	Compare(a, b Id) (cmp int, ok bool)

	// Is the first Id in this Rope before the other. For sorting.
	// Costs ~O(logn).
	Before(a, b Id) bool

	// Iter reads IDs from after the given Id.
	Iter(afterId Id) iter.Seq2[Id, T]

	// Insert data after the prior Id.
	// Length must be zero or positive.
	// Costs ~O(logn).
	InsertAfter(id Id, length int, data T) Id

	// DeleteTo deletes after the given ID until the target Id.
	// Pass zero/root for all content after.
	// Costs ~O(logn+m), where m is the number of nodes being deleted.
	DeleteTo(afterId, untilId Id)
}
