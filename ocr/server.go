package ocr

import (
	"github.com/samthor/thorgo/aatree"
	"github.com/samthor/thorgo/rope"
)

func New[Data any, Meta comparable]() ServerCr[Data, Meta] {
	rootNode := &internalNode[Data, Meta]{}
	r := rope.NewRoot[int](rootNode)
	idTree := aatree.New(func(a, b *internalNode[Data, Meta]) int { return a.id - b.id })
	idTree.Insert(rootNode)

	return &serverImpl[Data, Meta]{
		r:      r,
		idTree: idTree,
	}
}

type internalNode[Data any, Meta comparable] struct {
	id   int    // high id
	data []Data // data here (can only be nil at root)
	meta Meta
	del  bool
}

func (in *internalNode[Data, Meta]) len() int {
	if in.del {
		return 0
	}
	return len(in.data)
}

type serverImpl[Data any, Meta comparable] struct {
	len    int
	r      rope.Rope[int, *internalNode[Data, Meta]]
	idTree *aatree.AATree[*internalNode[Data, Meta]]
}

// lookupNode returns a valid node for the given ID, or nil if not possible.
// This can return the root node with zero length.
func (s *serverImpl[Data, Meta]) lookupNode(id int) (node *internalNode[Data, Meta], offset int) {
	lookup := internalNode[Data, Meta]{id: id}
	nearest, _ := s.idTree.EqualAfter(&lookup)
	if nearest == nil {
		return // no possible entry
	}

	offset = nearest.id - id
	if offset != 0 && offset >= len(nearest.data) {
		return
	}

	return nearest, offset
}

// lookupNodePair is a slight optimization over lookupNode when a node range is requested.
func (s *serverImpl[Data, Meta]) lookupNodePair(a, b int) (
	laNode *internalNode[Data, Meta],
	laOffset int,
	lbNode *internalNode[Data, Meta],
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

// ensureEdge ensures that there is a break at the given ID.
// Returns the new prior node, or nil if impossible.
func (s *serverImpl[Data, Meta]) ensureEdge(id int) bool {
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

	newNode := &internalNode[Data, Meta]{id: id, data: left, meta: nearest.meta, del: nearest.del}
	if !s.idTree.Insert(newNode) {
		panic("should insert new tree node")
	}

	// insert in reverse order
	ok1 := s.r.InsertIdAfter(prevId, nearest.id, nearest.len(), nearest)
	ok2 := s.r.InsertIdAfter(prevId, newNode.id, newNode.len(), newNode)
	if !ok1 || !ok2 {
		panic("should split fine")
	}

	return true
}

// maybeConsumeByAfter looks up the given node, and maybe deletes in favor of being consumed by its following node.
func (s *serverImpl[Data, Meta]) maybeConsumeByAfter(id int) (ok bool) {
	if id == 0 {
		return false
	}
	lookup := s.r.Info(id)
	if lookup.Id == 0 {
		// nb. used to panic, but we may call this multiple times in a move
		return
	} else if lookup.Next == 0 {
		return
	}

	rightLookup := s.r.Info(lookup.Next)
	if rightLookup.Id-len(rightLookup.Data.data) != lookup.Id ||
		rightLookup.Data.del != lookup.Data.del ||
		rightLookup.Data.meta != lookup.Data.meta {
		return
	}

	right := rightLookup.Data
	right.data = append(lookup.Data.data, right.data...)

	s.r.DeleteTo(lookup.Prev, lookup.Next)                           // delete both
	s.idTree.Remove(lookup.Data)                                     // delete left from idtree
	s.r.InsertIdAfter(lookup.Prev, right.id, len(right.data), right) // reinsert for new length

	return true
}

func (s *serverImpl[Data, Meta]) Len() int {
	return s.len
}

func (s *serverImpl[Data, Meta]) ReadAll() *SerializedState[Data, Meta] {
	var out SerializedState[Data, Meta]
	out.Data = make([]Data, 0, s.len)

	var lastId int

	for id, node := range s.r.Iter(0) {
		if node.Data.del {
			continue
		}

		out.Data = append(out.Data, node.Data.data...)

		delta := id - lastId
		out.Seq = append(out.Seq, len(node.Data.data), delta)
		out.Meta = append(out.Meta, node.Data.meta)
		lastId = id
	}
	return &out

}

func (s *serverImpl[Data, Meta]) EndSeq() int {
	return s.r.LastId()
}

func (s *serverImpl[Data, Meta]) ReconcileSeq(id int) (outId int, ok bool) {
	pos, ok := s.PositionFor(id)
	if !ok {
		return
	}
	return s.FindAt(pos), true
}

func (s *serverImpl[Data, Meta]) PositionFor(id int) (pos int, ok bool) {
	node, offset := s.lookupNode(id)
	if node == nil {
		return -1, false
	}
	nodePosition := s.r.Find(node.id)
	return nodePosition - offset, true
}

func (s *serverImpl[Data, Meta]) FindAt(at int) int {
	id, offset := s.r.ByPosition(at, false)
	return id - offset
}

func (s *serverImpl[Data, Meta]) Compare(a, b int) (cmp int, ok bool) {
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

func (s *serverImpl[Data, Meta]) LeftOf(id int) int {
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

func (s *serverImpl[Data, Meta]) PerformAppend(after, id int, data []Data, meta Meta) (deleted, ok bool) {
	l := len(data)
	if l == 0 {
		return // has no length
	}
	if check, _ := s.idTree.After(&internalNode[Data, Meta]{id: id - len(data)}); check != nil && check.id <= id {
		return // already exists
	}
	if !s.ensureEdge(after) {
		return // cannot create edge here
	}

	lookup := s.r.Info(after)
	shouldAppend := after != 0 && after == (id-len(data)) && lookup.Data.meta == meta

	var node *internalNode[Data, Meta]
	if shouldAppend {
		// we can modify left node directly; steal, remove and modify
		node = lookup.Data
		s.idTree.Remove(node)
		s.r.DeleteTo(lookup.Prev, after)
		node.id = id
		node.data = append(node.data, data...)
		after = lookup.Prev
	} else {
		// create brand-new node
		node = &internalNode[Data, Meta]{id: id, data: data, meta: meta}
	}

	// take deleted state from parent node
	node.del = lookup.Data.del

	s.idTree.Insert(node)
	s.r.InsertIdAfter(after, id, node.len(), node)
	s.len += l

	s.maybeConsumeByAfter(id) // possible but unlikely that we insert sequentially before another

	return node.del, true
}

func (s *serverImpl[Data, Meta]) PerformDelete(a, b int) (outA int, outB int, ok bool) {
	low, high, ok := s.boundaryFor(a, b)
	if !ok {
		return
	}

	ok1 := s.ensureEdge(low)
	ok2 := s.ensureEdge(high)
	if !ok1 || !ok2 {
		panic("can't ensureEdge for delete")
	}

	var deletedIds []int
	afterId := low

	for id, rn := range s.r.Iter(low) {
		if !rn.Data.del {
			rn.Data.del = true
			s.r.DeleteTo(afterId, id)
			s.r.InsertIdAfter(afterId, id, 0, rn.Data)

			deletedIds = append(deletedIds, id-len(rn.Data.data)+1, id) // store start-end of node (we take extent later)

			s.len -= len(rn.Data.data)
		}

		if id == high {
			break // boundaryFor prevents us from getting 'zero range'
		}
		afterId = id
	}

	if len(deletedIds) == 0 {
		return 0, 0, true
	}
	return deletedIds[0], deletedIds[len(deletedIds)-1], true
}

func (s *serverImpl[Data, Meta]) PerformMove(a, b int, afterId int) (outA, outB, effectiveAfter int, ok bool) {
	low, high, ok := s.boundaryFor(a, b)
	if !ok {
		return
	}

	var noopMove bool

	if cmp, _ := s.Compare(afterId, low); cmp >= 0 {
		if cmp, _ := s.Compare(afterId, high); cmp <= 0 {
			// we still do this move even though it's a no-op: helps with deleted out and 'user selects self'
			afterId = low
			noopMove = true
		}
	}

	hasAfter := s.ensureEdge(afterId)
	if !hasAfter {
		return // invalid target
	}
	originalAfterId := afterId
	// isDeleted := s.r.Info(originalAfterId).Data.del

	ok1 := s.ensureEdge(low)
	ok2 := s.ensureEdge(high)
	if !ok1 || !ok2 {
		panic("can't ensureEdge for move")
	}

	// precalc what is the valid 'after' before us by cheating: find position biasing before delete, then calc ID from that
	afterPos := s.r.Find(afterId)
	if afterPos == -1 {
		panic("could not find afterId previously edged")
	}
	positionId, positionOffset := s.r.ByPosition(afterPos, false)
	effectiveAfter = positionId - positionOffset

	var undeletedMoveIds []int
	for id, rn := range s.r.Iter(low) {
		if !rn.Data.del {
			undeletedMoveIds = append(undeletedMoveIds, id-len(rn.Data.data)+1, id)
		}

		if !noopMove {
			if s.r.DeleteTo(low, id) != 1 {
				panic("expected single delete")
			}

			// TODO: could restore to "delete if target deleted"
			// if isDeleted && !rn.Data.del {
			// 	rn.Data.del = true
			// 	s.len -= len(rn.Data.data)
			// }

			if !s.r.InsertIdAfter(afterId, id, rn.Data.len(), rn.Data) {
				panic("couldn't move node")
			}
		}

		if id == high {
			break // boundaryFor prevents us from getting 'zero range'
		}
		afterId = id
	}

	if len(undeletedMoveIds) != 0 {
		outA = undeletedMoveIds[0]
		outB = undeletedMoveIds[len(undeletedMoveIds)-1]
	}

	// check for sequential fixes:
	s.maybeConsumeByAfter(low)             // where we removed
	s.maybeConsumeByAfter(originalAfterId) // where we inserted
	s.maybeConsumeByAfter(high)            // the end of the insert

	ok = true
	return
}

// boundaryFor sorts the given target nodes and returns the edge before the lower one.
func (s *serverImpl[Data, Meta]) boundaryFor(a, b int) (low, high int, ok bool) {
	var cmp int
	cmp, ok = s.Compare(a, b)
	if !ok {
		return
	}
	if cmp > 0 {
		a, b = b, a
	}

	leftOf := s.LeftOf(a)
	if leftOf == -1 {
		return
	}

	low = leftOf
	high = b

	if low != high {
		ok = true
	}
	return
}
