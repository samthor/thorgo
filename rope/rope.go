package rope

import (
	"fmt"
	"iter"
	"log"
	"strings"
)

const (
	poolSize  = 8
	maxHeight = 32
)

// NewRoot builds a new Rope[ID, T] with a given root value for the zero ID.
func NewRoot[ID comparable, T any](root T) Rope[ID, T] {
	out := &ropeImpl[ID, T]{
		byID:     map[ID]*ropeNode[ID, T]{},
		height:   1,
		nodePool: make([]*ropeNode[ID, T], 0, poolSize),
	}
	out.head.dl.Data = root

	var zeroID ID
	out.byID[zeroID] = &out.head
	out.head.levels = make([]ropeLevel[ID, T], 1, maxHeight) // never alloc again
	out.head.levels[0] = ropeLevel[ID, T]{prev: &out.head}
	return out
}

// New builds a new Rope[ID, T].
func New[ID comparable, T any]() Rope[ID, T] {
	var root T
	return NewRoot[ID](root)
}

type ropeLevel[ID comparable, T any] struct {
	next        *ropeNode[ID, T] // can be nil
	prev        *ropeNode[ID, T] // always set
	subtreesize int
}

type iterRef[ID comparable, T any] struct {
	count int
	node  *ropeNode[ID, T]
}

type ropeNode[ID comparable, T any] struct {
	id     ID
	dl     DataLen[T]
	levels []ropeLevel[ID, T]

	// if set, an iterator is chilling here for the next value
	iterRef *iterRef[ID, T]
}

type ropeImpl[ID comparable, T any] struct {
	head     ropeNode[ID, T]
	len      int
	byID     map[ID]*ropeNode[ID, T]
	height   int // matches len(head.levels)
	nodePool []*ropeNode[ID, T]
	lastID   ID
}

func (r *ropeImpl[ID, T]) DebugPrint() {
	log.Printf("> rope len=%d heads=%d", r.len, r.height)
	const pipePart = "|     "
	const blankPart = "      "

	curr := &r.head
	renderHeight := r.height

	for {
		var parts []string

		// add level parts
		for i, l := range curr.levels {
			key := "+"
			if l.next == nil {
				key = "*"
				renderHeight = min(i, renderHeight)
			}

			s := fmt.Sprintf("%s%-5d", key, l.subtreesize)
			parts = append(parts, s)
		}

		// add blank/pipe parts
		for j := len(curr.levels); j < r.height; j++ {
			part := pipePart
			if j >= renderHeight {
				part = blankPart
			}
			parts = append(parts, part)
		}

		// add actual data
		parts = append(parts, "id="+r.toString(curr.id))
		parts = append(parts, r.toString(curr.dl.Data))

		// render
		log.Printf("- %s", strings.Join(parts, ""))

		// move to next
		curr = curr.levels[0].next
		if curr == nil {
			break
		}

		// render lines to break up the entries
		var lineParts []string
		for range renderHeight {
			lineParts = append(lineParts, pipePart)
		}
		log.Printf("  %s", strings.Join(lineParts, ""))

	}
}

// toString is a helper to render data, only for DebugPrint.
func (r *ropeImpl[ID, T]) toString(data any) string {
	type stringable interface {
		String() string
	}
	if s, ok := any(data).(stringable); ok {
		return s.String()
	}
	if s, ok := any(data).(string); ok {
		return s
	}
	if s, ok := any(data).(int); ok {
		return fmt.Sprintf("%d", s)
	}
	return ""
}

func (r *ropeImpl[ID, T]) Len() int {
	return r.len
}

func (r *ropeImpl[ID, T]) Count() int {
	return len(r.byID) - 1
}

func (r *ropeImpl[ID, T]) Find(id ID) int {
	e := r.byID[id]
	if e == nil {
		return -1
	}

	node := e
	var pos int

	for node != &r.head {
		link := len(node.levels) - 1
		node = node.levels[link].prev
		pos += node.levels[link].subtreesize
	}

	return pos + e.dl.Len
}

func (r *ropeImpl[ID, T]) Info(id ID) (out Info[ID, T]) {
	node := r.byID[id]
	if node == nil {
		return
	}

	out.DataLen = node.dl
	out.ID = node.id

	ol := &node.levels[0]
	out.Prev = ol.prev.id // we always have prev
	if ol.next != nil {
		out.Next = ol.next.id
	}
	return out
}

func (r *ropeImpl[ID, T]) ByPosition(position int, biasAfter bool) (id ID, offset int) {
	if position < 0 || (!biasAfter && position == 0) {
		return
	} else if position > r.len || (biasAfter && position == r.len) {
		return r.lastID, 0
	}

	e := &r.head
outer:
	for h := r.height - 1; h >= 0; h-- {
		// traverse this height while we can
		for position > e.levels[h].subtreesize {
			position -= e.levels[h].subtreesize

			next := e.levels[h].next
			if next == nil {
				continue outer
			}
			e = next
		}

		// if we bias to end, move as far forward as possible (even zero)
		for biasAfter && position >= e.levels[h].subtreesize && e.levels[h].next != nil {
			position -= e.levels[h].subtreesize
			e = e.levels[h].next
		}
	}

	return e.id, e.dl.Len - position

	// return e.levels[0].next.id, e.dl.Len - position
}

func (r *ropeImpl[ID, T]) InsertIDAfter(afterID, newID ID, length int, data T) bool {
	if length < 0 {
		panic("must be +ve len")
	}

	e := r.byID[afterID]
	if e == nil {
		return false // can't parent to another id
	}
	if _, ok := r.byID[newID]; ok {
		return false // id already exists
	}

	var height int
	var newNode *ropeNode[ID, T]
	var levels []ropeLevel[ID, T]

	if len(r.nodePool) != 0 {
		at := len(r.nodePool) - 1
		newNode = r.nodePool[at]
		r.nodePool = r.nodePool[:at]

		newNode.id = newID
		newNode.dl = DataLen[T]{Data: data, Len: length}
		levels = newNode.levels
		height = len(levels)

	} else {
		height = randomHeight()

		levels = make([]ropeLevel[ID, T], height)
		newNode = &ropeNode[ID, T]{
			dl:     DataLen[T]{Data: data, Len: length},
			id:     newID,
			levels: levels,
		}
	}
	r.byID[newID] = newNode

	// seek to see where it goes

	type ropeSeek[ID comparable, T any] struct {
		node *ropeNode[ID, T]
		sub  int
	}
	var seekStack [maxHeight]ropeSeek[ID, T] // using stack is 10-20% faster
	seek := seekStack[0:r.height]
	cseek := ropeSeek[ID, T]{
		node: e,
		sub:  e.dl.Len,
	}
	i := 0

	for {
		nl := len(cseek.node.levels)
		for i < nl {
			seek[i] = cseek
			i++
		}
		if cseek.node == &r.head || i == r.height {
			break
		}

		link := i - 1
		cseek.node = cseek.node.levels[link].prev
		cseek.sub += cseek.node.levels[link].subtreesize
	}

	// -- do actual insert

	for i = 0; i < height; i++ {
		if i < r.height {
			// we fit within head height (~99.9% of the time)
			n := seek[i].node
			nl := &n.levels[i]

			nextI := nl.next
			if nextI != nil {
				nextI.levels[i].prev = newNode
			}
			st := seek[i].sub

			levels[i] = ropeLevel[ID, T]{
				next:        nextI,
				prev:        n,
				subtreesize: length + nl.subtreesize - st,
			}

			nl.next = newNode
			nl.subtreesize = st

		} else {
			// this is a no-op on second go-around; we need to calc the actual insertPos for this
			// we previously gave up, `insertPos` was just the local consumed subtreesize
			link := len(cseek.node.levels) - 1
			for cseek.node != &r.head {
				cseek.node = cseek.node.levels[link].prev
				if len(cseek.node.levels)-1 != link {
					panic("inconsistent rope")
				}
				cseek.sub += cseek.node.levels[link].subtreesize
			}

			// ensure head has correct total height
			r.head.levels = append(r.head.levels, ropeLevel[ID, T]{
				next:        newNode,
				prev:        &r.head,
				subtreesize: cseek.sub,
			})
			r.height++

			levels[i] = ropeLevel[ID, T]{
				next:        nil,
				prev:        &r.head,
				subtreesize: r.len - cseek.sub + length,
			}
		}
	}

	for ; i < len(seek); i++ {
		node := seek[i].node
		node.levels[i].subtreesize += length
	}
	r.len += length

	if r.lastID == afterID {
		r.lastID = newID
	}

	return true
}

func (r *ropeImpl[ID, T]) rseekNodes(curr *ropeNode[ID, T], target *[maxHeight]*ropeNode[ID, T]) {
	i := 0
	for {
		ll := len(curr.levels)
		for i < ll {
			target[i] = curr
			i++
			if i == r.height {
				return
			}
		}
		curr = curr.levels[ll-1].prev
	}
}

func (r *ropeImpl[ID, T]) Less(a, b ID) bool {
	c, _ := r.Compare(a, b)
	return c < 0
}

func (r *ropeImpl[ID, T]) Between(afterA, afterB ID) (distance int, ok bool) {
	posA := r.Find(afterA)
	if posA < 0 {
		return
	}

	posB := r.Find(afterB)
	if posB < 0 {
		return
	}

	return posB - posA, true
}

func (r *ropeImpl[ID, T]) Compare(a, b ID) (cmp int, ok bool) {
	if a == b {
		_, ok = r.byID[a]
		return
	}

	anode := r.byID[a]
	bnode := r.byID[b]

	if anode == nil || bnode == nil {
		return
	}

	// this is about 15% faster than the naïve version (rseekNodes for both)
	// swapping might be a touch faster, maybe negligible

	cmp = 1
	ok = true
	if len(anode.levels) < len(bnode.levels) {
		// swap more levels into anode; seek will be faster
		cmp = -1
		anode, bnode = bnode, anode
	}

	curr := bnode

	var anodes [maxHeight]*ropeNode[ID, T]
	r.rseekNodes(anode, &anodes)

	// walk up the tree
	i := 1
	for {
		ll := len(curr.levels)
		for i < ll {
			// stepped "right" into the previous node tree, so it must be after us
			if curr == anodes[i] {
				return
			}
			i++
		}

		ll--
		curr = curr.levels[ll].prev
		if curr == anodes[ll] {
			// stepped "up" into the previous node tree, so must be before us
			cmp = -cmp
			return
		} else if curr == &r.head {
			// stepped "up" to root, so must be after us (we never saw it in walk)
			return
		}
	}
}

func (r *ropeImpl[ID, T]) DeleteTo(afterID, untilID ID) (count int) {
	lookup := r.byID[afterID]
	if lookup == nil {
		return
	}

	var nodes [maxHeight]*ropeNode[ID, T]
	r.rseekNodes(lookup, &nodes)

	prevLoopID := afterID

	for {
		e := nodes[0].levels[0].next
		if e == nil {
			r.lastID = afterID // we deleted to end, take last known good
			return
		}
		if prevLoopID == untilID {
			return
		}

		// if someone is/was iterating here, go _back_ so they'll start up again from after the previous node
		// this is probably a bit weird but it is an approach
		if e.iterRef != nil {
			e.iterRef.node = e.levels[0].prev
		}

		delete(r.byID, e.id)
		r.len -= e.dl.Len
		count++

		for i := range r.height {
			node := nodes[i]
			nl := &node.levels[i]
			if i >= len(e.levels) {
				// tail node
				nl.subtreesize -= e.dl.Len
				continue
			}

			// mid node 'before us'
			el := e.levels[i]
			nl.subtreesize += el.subtreesize - e.dl.Len
			c := el.next
			if c != nil {
				c.levels[i].prev = node
			}
			nl.next = c // when this becomes nil for levels[0], we bail
		}

		prevLoopID = e.id
		r.returnToPool(e) // clears id
	}
}

func (r *ropeImpl[ID, T]) returnToPool(e *ropeNode[ID, T]) {
	if len(r.nodePool) == poolSize || e.iterRef != nil {
		return
	}

	var zero ropeLevel[ID, T]
	for i := range e.levels {
		e.levels[i] = zero
	}

	// this just clears stuff in case it's a ptr for GC
	var tmp ID
	e.dl = DataLen[T]{}
	e.id = tmp

	r.nodePool = append(r.nodePool, e)
}

func (r *ropeImpl[ID, T]) Iter(afterID ID) iter.Seq2[ID, DataLen[T]] {
	return func(yield func(ID, DataLen[T]) bool) {
		e := r.byID[afterID]
		if e == nil {
			return
		}

		for {
			next := e.levels[0].next
			if next == nil {
				return
			}

			e = next

			if e.iterRef == nil {
				e.iterRef = &iterRef[ID, T]{node: e, count: 1}
			} else {
				e.iterRef.count++
			}

			shouldContinue := yield(e.id, e.dl)

			// this will probably be ourselves unless we were deleted
			update := e.iterRef.node
			e.iterRef.count--
			if e.iterRef.count == 0 {
				e.iterRef = nil
			}
			e = update

			if !shouldContinue {
				return
			}
		}
	}
}

func (r *ropeImpl[ID, T]) LastID() ID {
	return r.lastID
}
