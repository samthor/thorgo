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

// NewRoot builds a new Rope[Id, T] with a given root value for the zero ID.
func NewRoot[Id comparable, T any](root T) Rope[Id, T] {
	out := &ropeImpl[Id, T]{
		byId:     map[Id]*ropeNode[Id, T]{},
		height:   1,
		nodePool: make([]*ropeNode[Id, T], 0, poolSize),
	}
	out.head.dl.Data = root

	var zeroId Id
	out.byId[zeroId] = &out.head
	out.head.levels = make([]ropeLevel[Id, T], 1, maxHeight) // never alloc again
	out.head.levels[0] = ropeLevel[Id, T]{prev: &out.head}
	return out
}

// New builds a new Rope[Id, T].
func New[Id comparable, T any]() Rope[Id, T] {
	var root T
	return NewRoot[Id](root)
}

type ropeLevel[Id comparable, T any] struct {
	next        *ropeNode[Id, T] // can be nil
	prev        *ropeNode[Id, T] // always set
	subtreesize int
}

type iterRef[Id comparable, T any] struct {
	count int
	node  *ropeNode[Id, T]
}

type ropeNode[Id comparable, T any] struct {
	id     Id
	dl     DataLen[T]
	levels []ropeLevel[Id, T]

	// if set, an iterator is chilling here for the next value
	iterRef *iterRef[Id, T]
}

type ropeImpl[Id comparable, T any] struct {
	head     ropeNode[Id, T]
	len      int
	byId     map[Id]*ropeNode[Id, T]
	height   int // matches len(head.levels)
	nodePool []*ropeNode[Id, T]
	lastId   Id
}

func (r *ropeImpl[Id, T]) DebugPrint() {
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
func (r *ropeImpl[Id, T]) toString(data any) string {
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

func (r *ropeImpl[Id, T]) Len() int {
	return r.len
}

func (r *ropeImpl[Id, T]) Count() int {
	return len(r.byId) - 1
}

func (r *ropeImpl[Id, T]) Find(id Id) int {
	e := r.byId[id]
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

func (r *ropeImpl[Id, T]) Info(id Id) (out Info[Id, T]) {
	node := r.byId[id]
	if node == nil {
		return
	}

	out.DataLen = node.dl
	out.Id = node.id

	ol := &node.levels[0]
	out.Prev = ol.prev.id // we always have prev
	if ol.next != nil {
		out.Next = ol.next.id
	}
	return out
}

func (r *ropeImpl[Id, T]) ByPosition(position int, biasAfter bool) (id Id, offset int) {
	if position < 0 || (!biasAfter && position == 0) {
		return
	} else if position > r.len || (biasAfter && position == r.len) {
		return r.lastId, 0
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

func (r *ropeImpl[Id, T]) InsertIdAfter(afterId, newId Id, length int, data T) bool {
	if length < 0 {
		panic("must be +ve len")
	}

	e := r.byId[afterId]
	if e == nil {
		return false // can't parent to another id
	}
	if _, ok := r.byId[newId]; ok {
		return false // id already exists
	}

	var height int
	var newNode *ropeNode[Id, T]
	var levels []ropeLevel[Id, T]

	if len(r.nodePool) != 0 {
		at := len(r.nodePool) - 1
		newNode = r.nodePool[at]
		r.nodePool = r.nodePool[:at]

		newNode.id = newId
		newNode.dl = DataLen[T]{Data: data, Len: length}
		levels = newNode.levels
		height = len(levels)

	} else {
		height = randomHeight()

		levels = make([]ropeLevel[Id, T], height)
		newNode = &ropeNode[Id, T]{
			dl:     DataLen[T]{Data: data, Len: length},
			id:     newId,
			levels: levels,
		}
	}
	r.byId[newId] = newNode

	// seek to see where it goes

	type ropeSeek[Id comparable, T any] struct {
		node *ropeNode[Id, T]
		sub  int
	}
	var seekStack [maxHeight]ropeSeek[Id, T] // using stack is 10-20% faster
	seek := seekStack[0:r.height]
	cseek := ropeSeek[Id, T]{
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

			levels[i] = ropeLevel[Id, T]{
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
			r.head.levels = append(r.head.levels, ropeLevel[Id, T]{
				next:        newNode,
				prev:        &r.head,
				subtreesize: cseek.sub,
			})
			r.height++

			levels[i] = ropeLevel[Id, T]{
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

	if r.lastId == afterId {
		r.lastId = newId
	}

	return true
}

func (r *ropeImpl[Id, T]) rseekNodes(curr *ropeNode[Id, T], target *[maxHeight]*ropeNode[Id, T]) {
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

func (r *ropeImpl[Id, T]) Less(a, b Id) bool {
	c, _ := r.Compare(a, b)
	return c < 0
}

func (r *ropeImpl[Id, T]) Between(afterA, afterB Id) (distance int, ok bool) {
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

func (r *ropeImpl[Id, T]) Compare(a, b Id) (cmp int, ok bool) {
	if a == b {
		_, ok = r.byId[a]
		return
	}

	anode := r.byId[a]
	bnode := r.byId[b]

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

	var anodes [maxHeight]*ropeNode[Id, T]
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

func (r *ropeImpl[Id, T]) DeleteTo(afterId, untilId Id) (count int) {
	lookup := r.byId[afterId]
	if lookup == nil {
		return
	}

	var nodes [maxHeight]*ropeNode[Id, T]
	r.rseekNodes(lookup, &nodes)

	prevLoopId := afterId

	for {
		e := nodes[0].levels[0].next
		if e == nil {
			r.lastId = afterId // we deleted to end, take last known good
			return
		}
		if prevLoopId == untilId {
			return
		}

		// if someone is/was iterating here, go _back_ so they'll start up again from after the previous node
		// this is probably a bit weird but it is an approach
		if e.iterRef != nil {
			e.iterRef.node = e.levels[0].prev
		}

		delete(r.byId, e.id)
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

		prevLoopId = e.id
		r.returnToPool(e) // clears id
	}
}

func (r *ropeImpl[Id, T]) returnToPool(e *ropeNode[Id, T]) {
	if len(r.nodePool) == poolSize || e.iterRef != nil {
		return
	}

	var zero ropeLevel[Id, T]
	for i := range e.levels {
		e.levels[i] = zero
	}

	// this just clears stuff in case it's a ptr for GC
	var tmp Id
	e.dl = DataLen[T]{}
	e.id = tmp

	r.nodePool = append(r.nodePool, e)
}

func (r *ropeImpl[Id, T]) Iter(afterId Id) iter.Seq2[Id, DataLen[T]] {
	return func(yield func(Id, DataLen[T]) bool) {
		e := r.byId[afterId]
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
				e.iterRef = &iterRef[Id, T]{node: e, count: 1}
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

func (r *ropeImpl[Id, T]) LastId() Id {
	return r.lastId
}
