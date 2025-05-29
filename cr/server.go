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

func (s *serverImpl[Data, Meta]) PerformDelete(from, until int) (a, b int, ok bool) {
	if cmp, _ := s.ca.Compare(from, until); cmp > 0 {
		until, from = from, until
	}

	// unlike this call, Mark does work on "boundaries", so we move to the left by one
	leftOf := s.ca.LeftOf(from)
	if leftOf == -1 {
		return
	}
	newlyIncluded, _, ok := s.ro.Mark(leftOf, until)
	if !ok {
		return
	}

	if len(newlyIncluded) != 0 {
		a = s.ca.RightOf(newlyIncluded[0])
		if a == -1 {
			panic("invalid RightOf")
		}
		b = newlyIncluded[len(newlyIncluded)-1]
	}
	return a, b, true
}

func (s serverImpl[Data, Meta]) LastSeq() int {
	return s.ca.r.LastId()
}

func (s *serverImpl[Data, Meta]) Read(a, b int) *ServerCrState[Data, Meta] {
	var state ServerCrState[Data, Meta]
	var parts [][]Data
	var dataLen int
	var lastId int

	for ia, ib := range s.ro.ValidIter(a, b) {
		for part := range s.ca.Read(ia, ib) {
			parts = append(parts, part.data)
			state.Meta = append(state.Meta, part.meta)
			dataLen += len(part.data)

			delta := part.id - lastId
			state.Seq = append(state.Seq, len(part.data), delta)
			lastId = part.id
		}
	}

	state.Data = make([]Data, 0, dataLen)
	for _, p := range parts {
		state.Data = append(state.Data, p...)
	}

	return &state
}
