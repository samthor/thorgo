package cr

import (
	"iter"
)

type ServerCr[Data any, Meta comparable] interface {
	Len() int
	Iter() iter.Seq2[int, []Data]

	// PerformAppend inserts data into this ServerCr after the prior node.
	// Returns true if the data was inserted.
	// As a convenience, returns the new ID of the data.
	PerformAppend(after int, data []Data, meta Meta) (now int, ok bool)
}
