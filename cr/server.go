package cr

type ServerCr[Data any, Meta comparable] interface {
	Len() int

	// HighSeq returns the high node ID.
	// Will be zero at start.
	HighSeq() int

	// PositionFor returns the position for the given ID.
	PositionFor(id int) int

	// PerformAppend inserts data after the prior node.
	// Returns true if the data was inserted, but false if the prior node could not be found.
	// Returns the new ID of the data. (TODO: doesn't need to)
	PerformAppend(after int, data []Data, meta Meta) (now int, ok bool)

	// PerformDelete marks the given range as deleted.
	// It does not actually delete anything.
	PerformDelete(after, until int) (delta int, ok bool)
}

type serverImpl[Data any, Meta comparable] struct {
	ca *crAddImpl[Data, Meta]
	ro *rangeOver[int]
}

// New creates a new ServerCr.
// TODO: it can't really be _used_ yet
func New[Data any, Meta comparable]() ServerCr[Data, Meta] {
	ca := newCrAdd[Data, Meta]()
	ro := newRange(ca.r)
	return &serverImpl[Data, Meta]{ca: ca, ro: ro}
}

func (s *serverImpl[Data, Meta]) Len() int {
	return s.ca.Len() - s.ro.Delta()
}

func (s *serverImpl[Data, Meta]) HighSeq() int {
	return s.ca.highSeq
}

func (s *serverImpl[Data, Meta]) PositionFor(id int) int {
	return s.ca.PositionFor(id) - s.ro.DeltaFor(id)
}

func (s *serverImpl[Data, Meta]) PerformAppend(after int, data []Data, meta Meta) (now int, ok bool) {
	now, ok = s.ca.PerformAppend(after, data, meta)
	if ok {
		s.ro.Grow(after, len(data))
	}
	return
}

func (s *serverImpl[Data, Meta]) PerformDelete(after, until int) (delta int, ok bool) {
	_, delta, ok = s.ro.Mark(after, until)
	return
}
