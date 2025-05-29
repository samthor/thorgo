package cr

type serverImpl[Data any, Meta comparable] struct {
	ca *crAddImpl[Data, Meta]
	ro *rangeOver[int]
}

// New creates a new ServerCr.
// TODO: it can't really be _used_ yet
func New[Data any, Meta comparable]() ServerCr[Data, Meta] {
	out := &serverImpl[Data, Meta]{ca: newCrAdd[Data, Meta]()}
	out.ro = newRange(out.ca)
	return out
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

func (s *serverImpl[Data, Meta]) PerformAppend(after int, data []Data, meta Meta) (deleted, ok bool) {
	now, ok := s.ca.PerformAppend(after, data, meta)
	if ok {
		deleted = s.ro.Grow(after, len(data), now)
	}
	return deleted, ok
}

func (s *serverImpl[Data, Meta]) PerformDelete(from, until int) (delta int, ok bool) {
	if cmp, _ := s.ca.Compare(from, until); cmp > 0 {
		until, from = from, until
	}

	// unlike this call, Mark does work on "boundaries", so we move to the left by one
	leftOf := s.ca.LeftOf(from)
	if leftOf == -1 {
		return
	}
	_, delta, ok = s.ro.Mark(leftOf, until)
	return
}

// Serialize returns data ready for a "normal" client to use, with no deleted data.
func (s *serverImpl[Data, Meta]) Serialize() *ServerCrState[Data] {
	var seq []int
	var parts [][]Data
	var dataLen int
	var lastId int

	for a, b := range s.ro.ValidIter(0, s.ca.r.LastId()) {
		for part := range s.ca.Read(a, b) {
			parts = append(parts, part.data)
			dataLen += len(part.data)

			delta := part.id - lastId
			seq = append(seq, len(part.data), delta)
			lastId = part.id
		}
	}

	out := make([]Data, 0, dataLen)
	for _, p := range parts {
		out = append(out, p...)
	}
	return &ServerCrState[Data]{Data: out, Seq: seq}
}
