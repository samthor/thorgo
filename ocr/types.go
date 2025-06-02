package ocr

type ServerCr[Data any, Meta comparable] interface {
	Len() int

	// Read flattens this state for use by end users.
	// This will not include deleted nodes.
	ReadAll() *SerializedState[Data, Meta]

	// EndSeq returns the node ID at the end of this data.
	// This may be a deleted ID and not normally visible.
	// Use this as part of Read to read all data.
	EndSeq() int

	// ReconcileSeq returns the closest undeleted ID for the given ID.
	ReconcileSeq(id int) (outId int, ok bool)

	// PositionFor returns the position for the given ID.
	PositionFor(id int) (position int, ok bool)

	// FindAt returns the ID for the given position in the data.
	// This always returns a valid ID as it is clamped by the length.
	FindAt(at int) int

	// Compare compares the position of the two IDs.
	Compare(a, b int) (cmp int, ok bool)

	// PerformAppend inserts data after the prior node.
	// Specify its ID, which is the tail of the data, and all data in the sequence must have a unique ID.
	PerformAppend(after, id int, data []Data, meta Meta) (deleted, ok bool)

	// PerformDelete marks the given range as deleted.
	// Both arguments point directly to nodes, so it is valid for both values to be equal (and deletion of "one" will occur).
	// Returns the newly deleted range, which may be less than given (deleting within already deleted).
	// If there's no newly deleted range, the range is [0,0], but this can still be 'ok'.
	PerformDelete(a, b int) (outA, outB int, ok bool)

	// PerformMove moves the given range to after another node.
	// If the other node is within the range itself, this is a no-op.
	// It has the same semantics as PerformDelete: point _at_ nodes, not before nodes.
	// This does not change the deleted state of the moved nodes (even e.g., if moving undeleted after deleted).
	// Returns the start/end of the non-deleted moved range, and the last non-deleted ID that this is after.
	PerformMove(a, b, after int) (outA, outB, effectiveAfter int, ok bool)
}

type SerializedState[Data, Meta any] struct {
	Data []Data `json:"data"` // underlying data in run stuck together
	Seq  []int  `json:"seq"`  // pairs of [length,seqDelta]
	Meta []Meta `json:"-"`    // meta of data, always half of Seq
}
