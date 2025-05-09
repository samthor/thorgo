// package rope implements a skip list.

package rope

type Id int

type Info[T any] struct {
	Id, Next, Prev Id
	Length         int
	Data           T
}

type Rope[T any] interface {

	// Returns the total sum of the parts of the rope. O(1).
	Len() int

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

	// Insert data after the prior Id.
	// Costs ~O(logn).
	InsertAfter(id Id, data T, length int) Id

	// DeleteTo deletes after the given ID until the target Id.
	// Pass zero/root for all content after.
	//  Costs ~O(logn+m), where m is the number of nodes being deleted.
	DeleteTo(afterId, untilId Id)
}
