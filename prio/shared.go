package prio

type Less[X Less[X]] interface {
	// Less checks if this item is before the passed item.
	//
	// This attaches to the generic, but has the same meaning as the sort package.
	//
	// TODO: clearly cmp.Ordered is probably faster. Stupid Go.
	Less(other X) (is bool)
}

type KeyQueue[K comparable, P Less[P]] interface {
	// Add inserts this K with the given priority P.
	// If the K already exists, this re-orders it if the priority has changed.
	Add(k K, p P) (anew bool)

	// Delete deletes this K, returning true if a change was made.
	Delete(k K) (ok bool)

	// Next removes an item from the front of this KeyQueue.
	// It retuns the zero values if this is empty.
	Next() (k K, p P)

	// Peek is as Next, but does not actually remove the value.
	Peek() (k K, p P)

	// Len returns the number of items here.
	Len() (length int)

	// All copies the data into an output slice.
	// This is not fast, and is for testing.
	All() (out []K, prio []P)
}
