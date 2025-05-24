package cr

import (
	"iter"
	"log"

	"github.com/samthor/thorgo/aatree"
	"github.com/samthor/thorgo/rope"
)

func New[Data any, Meta comparable]() ServerCr[Data, Meta] {
	return &serverCrImpl[Data, Meta]{
		r:      rope.New[int, *internalNode[[]Data, Meta]](),
		idTree: aatree.New(func(a, b *internalNode[[]Data, Meta]) int { return a.id - b.id }),
	}
}

type internalNode[Data any, Meta comparable] struct {
	id   int // high ID
	data Data
	meta Meta
	// del  int
}

type serverCrImpl[Data any, Meta comparable] struct {
	len     int
	highSeq int
	r       rope.Rope[int, *internalNode[[]Data, Meta]]
	idTree  *aatree.AATree[*internalNode[[]Data, Meta]]
}

func (s *serverCrImpl[Data, Meta]) ensureEdge(id int) bool {
	if id == 0 {
		return true
	} else if id < 0 {
		return false
	}

	nearest, _ := s.idTree.EqualAfter(&internalNode[[]Data, Meta]{id: id})
	if nearest == nil {
		log.Printf("we don't exist: %v", id)
		return false // no possible entry
	}

	at := len(nearest.data) - (nearest.id - id)
	if at < 0 {
		return false // past the nearest's size, we don't exist
	} else if at == len(nearest.data) {
		return true // we're on an edge already
	}

	lookup := s.r.Info(nearest.id)
	prevId := lookup.Prev

	count := s.r.DeleteTo(prevId, nearest.id)
	if count != 1 {
		panic("should delete just found node")
	}

	left := nearest.data[0:at]
	nearest.data = nearest.data[at:]
	newNode := &internalNode[[]Data, Meta]{id: id, data: left, meta: nearest.meta}

	// insert in reverse order
	ok1 := s.r.InsertIdAfter(prevId, nearest.id, len(nearest.data), nearest)
	ok2 := s.r.InsertIdAfter(prevId, newNode.id, len(newNode.data), newNode)

	s.idTree.Insert(nearest)
	s.idTree.Insert(newNode)

	if !ok1 || !ok2 {
		panic("should split fine")
	}

	return true
}

func (s *serverCrImpl[Data, Meta]) maybeMergeWithLeft(id int) {
	lookup := s.r.Info(id)
	if lookup.Id == 0 {
		panic("can't find node for maybeMergeWithLeft")
	} else if lookup.Prev == 0 {
		return
	}
	right := lookup.Data

	// make sure we're in the right spot and our meta is the same
	leftLookup := s.r.Info(lookup.Prev)
	if leftLookup.Id != lookup.Id-len(right.data) || leftLookup.Data.meta != right.meta {
		return
	}
	right.data = append(leftLookup.Data.data, right.data...)

	// delete both from rope
	s.r.DeleteTo(leftLookup.Prev, lookup.Id)

	// delete left from idtree
	s.idTree.Remove(leftLookup.Data)

	// reinsert right into rope
	s.r.InsertIdAfter(leftLookup.Prev, right.id, len(right.data), right)
}

func (s *serverCrImpl[Data, Meta]) Len() int {
	return s.len
}

func (s *serverCrImpl[Data, Meta]) PerformAppend(after int, data []Data, meta Meta) (now int, ok bool) {
	l := len(data)
	if l == 0 || !s.ensureEdge(after) {
		return 0, false
	}

	s.highSeq += l
	id := s.highSeq

	node := &internalNode[[]Data, Meta]{id: id, data: data, meta: meta}
	ok = s.r.InsertIdAfter(after, id, l, node)
	if !ok {
		panic("couldn't insertIdAfter even after edge split")
	}
	s.idTree.Insert(node)
	s.len += len(data)

	s.maybeMergeWithLeft(id)

	return id, ok
}

func (s *serverCrImpl[Data, Meta]) Iter() iter.Seq2[int, []Data] {
	return func(yield func(int, []Data) bool) {
		inner := s.r.Iter(0)
		inner(func(id int, dl rope.DataLen[*internalNode[[]Data, Meta]]) bool {
			return yield(id, dl.Data.data)
		})
	}
}
