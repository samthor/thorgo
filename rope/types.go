// package rope implements a skip list.

package rope

import (
	"iter"
)

// Info is a holder for info looked up in a Rope.
type Info[ID comparable, T any] struct {
	ID, Next, Prev ID
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
// The zero ID is always part of the Rope and has zero length, don't use it to add items.
type Rope[ID comparable, T any] interface {
	DebugPrint()

	// Returns the total sum of the parts of the rope. O(1).
	Len() (length int)

	// Returns the number of parts here. O(1).
	Count() (count int)

	// Finds the position after the given ID.
	// If the ID is not here, returns -1.
	// This lookup costs ~O(logn).
	Find(id ID) (position int)

	// Finds info on the given ID.
	// If the ID is not here, returns a zero Info struct.
	// This lookup costs O(1).
	Info(id ID) (out Info[ID, T])

	// Finds the ID/info at the position in the Rope.
	// Returns the offset from the end of the ID.
	// This costs ~O(logn).
	// Either stops before or skips after zero-length content based on biasAfter.
	// e.g., with 0/false, this will aways return the zero ID.
	ByPosition(position int, biasAfter bool) (id ID, offset int)

	// Between returns the distance between _after_ these two nodes.
	// This costs ~O(logn), and is more expensive than Compare.
	Between(afterA, afterB ID) (distance int, ok bool)

	// Compare the position of the two ID in this Rope.
	// Costs ~O(logn).
	Compare(a, b ID) (cmp int, ok bool)

	// Less determines if the first ID in this Rope before the other. For sorting.
	// Costs ~O(logn).
	Less(a, b ID) (less bool)

	// Iter reads from after the given ID.
	// It is safe to use even if the Rope is modified.
	Iter(afterID ID) (i iter.Seq2[ID, DataLen[T]])

	// Inserts a new entry after the prior ID.
	// This will panic if the length is negative.
	// Returns false if this was not possible (no parent, ID already exists).
	InsertIDAfter(afterID, id ID, length int, data T) (ok bool)

	// DeleteTo deletes after the given ID until the target ID.
	// Pass zero/root for all content after.
	// Costs ~O(logn+m), where m is the number of nodes being deleted.
	// Returns the number of nodes deleted.
	DeleteTo(afterID, untilID ID) (count int)

	// LastID returns the last ID, i.e., at the final position, in this rope.
	LastID() (id ID)
}
