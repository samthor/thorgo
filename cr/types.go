package cr

type ServerCr[Data any, Meta comparable] interface {
	Len() int

	// Serialize flattens this state for use by end users.
	Serialize() *ServerCrState[Data]

	// HighSeq returns the high node ID.
	// Will be zero at start.
	HighSeq() int

	// PositionFor returns the position for the given ID.
	PositionFor(id int) int

	// PerformAppend inserts data after the prior node.
	PerformAppend(after int, data []Data, meta Meta) (deleted, ok bool)

	// PerformDelete marks the given range as deleted.
	// Both arguments point directly to nodes, so it is valid for both values to be equal (and deletion of "one" will occur).
	PerformDelete(from, until int) (delta int, ok bool)
}

type ServerCrState[Data any] struct {
	Data []Data // underlying data in run
	Seq  []int  // pairs of [length,seqDelta]
}
