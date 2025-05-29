package cr

type ServerCr[Data any, Meta comparable] interface {
	Len() int

	// Read flattens this state for use by end users.
	Read(a, b int) *ServerCrState[Data, Meta]

	// LastSeq returns the last node ID.
	// This may be a deleted ID and not normally visible.
	LastSeq() int

	// HighSeq returns the high node ID.
	// Will be zero at start.
	HighSeq() int

	// PositionFor returns the position for the given ID.
	PositionFor(id int) int

	// PerformAppend inserts data after the prior node.
	// Use HighSeq to return its new ID.
	PerformAppend(after int, data []Data, meta Meta) (deleted, ok bool)

	// PerformDelete marks the given range as deleted.
	// Both arguments point directly to nodes, so it is valid for both values to be equal (and deletion of "one" will occur).
	// Returns the newly deleted range, which may be less than given (deleting within already deleted).
	// If there's no newly deleted range, the range is [0,0], but this can still be 'ok'.
	PerformDelete(from, until int) (a, b int, ok bool)
}

type ServerCrState[Data, Meta any] struct {
	Data []Data // underlying data in run stuck together
	Seq  []int  // pairs of [length,seqDelta]
	Meta []Meta // meta of data, always half of Seq
}
