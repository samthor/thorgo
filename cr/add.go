package cr

import (
	"iter"

	"github.com/samthor/thorgo/aatree"
	"github.com/samthor/thorgo/rope"
)

func newCrAdd[Data any, Meta comparable]() *crAddImpl[Data, Meta] {
	rope := rope.New[int, *internalNode[[]Data, Meta]]()

	idTree := aatree.New(func(a, b *internalNode[[]Data, Meta]) int { return a.id - b.id })
	idTree.Insert(&internalNode[[]Data, Meta]{}) // zero node (id=0 etc)

	return &crAddImpl[Data, Meta]{
		r:      rope,
		idTree: idTree,
	}
}

type internalNode[Data any, Meta comparable] struct {
	id   int // high ID
	data Data
	meta Meta
}

type crAddImpl[Data any, Meta comparable] struct {
	len     int
	highSeq int
	r       rope.Rope[int, *internalNode[[]Data, Meta]]
	idTree  *aatree.AATree[*internalNode[[]Data, Meta]]
}

func (s *crAddImpl[Data, Meta]) ensureEdge(id int) bool {
	nearest, offset := s.lookupNode(id)
	if nearest == nil {
		return false // no possible entry
	} else if offset == 0 {
		return true // we're on an edge already
	}

	lookup := s.r.Info(nearest.id)
	prevId := lookup.Prev

	count := s.r.DeleteTo(prevId, nearest.id)
	if count != 1 {
		panic("should delete just found node")
	}

	at := len(nearest.data) - offset
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

// TODO: currently disused since we modify _directly_ on insert
func (s *crAddImpl[Data, Meta]) maybeMergeWithLeft(id int) {
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

func (s *crAddImpl[Data, Meta]) Len() int {
	return s.len
}

func (s *crAddImpl[Data, Meta]) PositionFor(id int) int {
	node, offset := s.lookupNode(id)
	if node == nil {
		return -1
	}

	// finds the START of this id
	nodePosition := s.r.Find(node.id)
	return nodePosition + len(node.data) - offset
}

func (s *crAddImpl[Data, Meta]) PerformAppend(after int, data []Data, meta Meta) (now int, ok bool) {
	l := len(data)
	if l == 0 {
		return
	}

	if !s.ensureEdge(after) {
		return 0, false
	}
	lookup := s.r.Info(after)
	shouldAppend := after != 0 && after == s.highSeq && lookup.Data.meta == meta

	s.len += l
	s.highSeq += l
	id := s.highSeq

	if shouldAppend {
		// we can modify left node directly
		node := lookup.Data

		// remove old
		s.idTree.Remove(node)
		s.r.DeleteTo(lookup.Prev, after)

		node.id = id
		node.data = append(node.data, data...)

		// append new
		s.idTree.Insert(node)
		s.r.InsertIdAfter(lookup.Prev, id, len(node.data), node)

	} else {
		// insert new node
		node := &internalNode[[]Data, Meta]{id: id, data: data, meta: meta}
		ok = s.r.InsertIdAfter(after, id, l, node)
		if !ok {
			panic("couldn't insertIdAfter even after edge split")
		}
		s.idTree.Insert(node)

	}

	return id, true
}

func (s *crAddImpl[Data, Meta]) Iter() iter.Seq2[int, []Data] {
	return func(yield func(int, []Data) bool) {
		inner := s.r.Iter(0)
		inner(func(id int, dl rope.DataLen[*internalNode[[]Data, Meta]]) bool {
			return yield(id, dl.Data.data)
		})
	}
}

func (s *crAddImpl[Data, Meta]) Read(a, b int) iter.Seq[crRead[Data, Meta]] {
	return func(yield func(crRead[Data, Meta]) bool) {
		startNode, startOffset, endNode, endOffset, ok := s.lookupNodePair(a, b)
		if !ok {
			return
		}

		// special-case single node
		if startNode == endNode {
			if endOffset >= startOffset {
				return // nothing
			}
			n := startNode
			l := len(n.data)
			slice := n.data[l-startOffset : l-endOffset]

			yield(crRead[Data, Meta]{
				id:   n.id - endOffset,
				data: slice,
				meta: n.meta,
			})
			return
		}

		// nodes might be in wrong order
		if cmp, _ := s.r.Compare(startNode.id, endNode.id); cmp > 0 {
			return
		}

		// yield start
		startData := startNode.data[len(startNode.data)-startOffset:]
		if len(startData) != 0 && !yield(crRead[Data, Meta]{
			id:   startNode.id,
			data: startData,
			meta: startNode.meta,
		}) {
			return
		}

		// yield mid nodes
		for id, dl := range s.r.Iter(startNode.id) {
			if id == endNode.id {
				break
			}
			if !yield(crRead[Data, Meta]{
				id:   id,
				data: dl.Data.data,
				meta: dl.Data.meta,
			}) {
				return
			}
		}

		// yield end
		yield(crRead[Data, Meta]{
			id:   endNode.id - endOffset,
			data: endNode.data[:len(endNode.data)-endOffset],
			meta: endNode.meta,
		})
	}
}

type crRead[Data, Meta any] struct {
	id   int    // end seq
	data []Data // underlying data
	meta Meta
}

func (s *crAddImpl[Data, Meta]) lookupNode(id int) (node *internalNode[[]Data, Meta], offset int) {
	nearest, _ := s.idTree.EqualAfter(&internalNode[[]Data, Meta]{id: id})
	if nearest == nil {
		return // no possible entry
	}

	offset = nearest.id - id
	if offset != 0 && offset >= len(nearest.data) {
		return
	}

	return nearest, offset
}

func (s *crAddImpl[Data, Meta]) lookupNodePair(a, b int) (
	laNode *internalNode[[]Data, Meta],
	laOffset int,
	lbNode *internalNode[[]Data, Meta],
	lbOffset int,
	ok bool,
) {
	laNode, laOffset = s.lookupNode(a)
	if laNode == nil {
		return
	}

	lbOffset = laNode.id - b
	if lbOffset >= 0 && lbOffset < len(laNode.data) {
		lbNode = laNode
		ok = true
		return
	}

	lbNode, lbOffset = s.lookupNode(b)
	if lbNode == nil {
		return
	}

	ok = true
	return
}

func (s *crAddImpl[Data, Meta]) Between(a, b int) (distance int, ok bool) {
	laNode, laOffset, lbNode, lbOffset, ok := s.lookupNodePair(a, b)
	if !ok {
		return
	}

	localOffset := laOffset - lbOffset
	if laNode == lbNode {
		return localOffset, true
	}

	distance, ok = s.r.Between(laNode.id, lbNode.id)
	distance += localOffset
	return
}

func (s *crAddImpl[Data, Meta]) Compare(a, b int) (cmp int, ok bool) {
	laNode, laOffset, lbNode, lbOffset, ok := s.lookupNodePair(a, b)
	if !ok {
		return
	}

	// nb. the result is always negative compared to Between

	if laNode == lbNode {
		return lbOffset - laOffset, true
	}

	return s.r.Compare(laNode.id, lbNode.id)
}

func (s *crAddImpl[Data, Meta]) LeftOf(id int) int {
	if id <= 0 {
		return -1
	}

	node, offset := s.lookupNode(id)
	if node == nil {
		return -1
	}

	if offset < len(node.data)-1 {
		return id - 1
	}
	return s.r.Info(node.id).Prev
}
