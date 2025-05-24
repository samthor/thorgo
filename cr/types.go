package cr

import (
	"iter"
)

type ServerCr[Data any, Meta comparable] interface {
	Len() int
	Iter() iter.Seq2[int, []Data]

	// PositionFor returns the position for the given ID.
	PositionFor(id int) int

	// PerformAppend inserts data into this ServerCr after the prior node.
	// Returns true if the data was inserted.
	// As a convenience, returns the new ID of the data.
	PerformAppend(after int, data []Data, meta Meta) (now int, ok bool)

	// PerformDelete(a, b int) (ok bool)
}
