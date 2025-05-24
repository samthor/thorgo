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

// New builds a new Rope[Id, T].
func New[Id comparable, T any]() Rope[Id, T] {
	out := &ropeImpl[Id, T]{
		byId:     map[Id]*ropeNode[Id, T]{},
		height:   1,
		nodePool: make([]*ropeNode[Id, T], 0, poolSize),
	}

	var zeroId Id
	out.byId[zeroId] = &out.head
	out.head.levels = make([]ropeLevel[Id, T], 1, maxHeight) // never alloc again
	out.head.levels[0] = ropeLevel[Id, T]{prev: &out.head}
	return out
}

type ropeLevel[Id comparable, T any] struct {
	next        *ropeNode[Id, T] // can be nil
	prev        *ropeNode[Id, T] // always set
	subtreesize int
}

type ropeNode[Id comparable, T any] struct {
	id     Id
	len    int
	levels []ropeLevel[Id, T]
	data   T
}

type ropeImpl[Id comparable, T any] struct {
	head     ropeNode[Id, T]
	len      int
	byId     map[Id]*ropeNode[Id, T]
	height   int // matches len(head.levels)
	nodePool []*ropeNode[Id, T]
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
		parts = append(parts, r.toString(curr.data))

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
func (r *ropeImpl[Id, T]) toString(data T) string {
	type stringable interface {
		String() string
	}
	if s, ok := any(data).(stringable); ok {
		return s.String()
	}
	if s, ok := any(data).(string); ok {
		return s
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

	return pos
}

func (r *ropeImpl[Id, T]) Info(id Id) (out Info[Id, T]) {
	node := r.byId[id]
	if node == nil {
		return
	}

	out.Data = node.data
	out.Length = node.len
	out.Id = node.id

	ol := &node.levels[0]
	out.Prev = ol.prev.id // we always have prev
	if ol.next != nil {
		out.Next = ol.next.id
	}
	return out
}

func (r *ropeImpl[Id, T]) ByPosition(position int, biasAfter bool) (id Id, offset int) {
	if position < 0 || position > r.len {
		offset = position
		return
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

	return e.id, position
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
		newNode.data = data
		newNode.len = length
		levels = newNode.levels
		height = len(levels)

	} else {
		height = randomHeight()

		levels = make([]ropeLevel[Id, T], height)
		newNode = &ropeNode[Id, T]{
			data:   data,
			id:     newId,
			len:    length,
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
		sub:  e.len,
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

func (r *ropeImpl[Id, T]) DeleteTo(afterId, untilId Id) {
	lookup := r.byId[afterId]
	if lookup == nil {
		return
	}

	var nodes [maxHeight]*ropeNode[Id, T]
	r.rseekNodes(lookup, &nodes)

	for {
		e := nodes[0].levels[0].next
		if e == nil {
			return
		}

		delete(r.byId, e.id)
		r.len -= e.len

		for i := range r.height {
			node := nodes[i]
			nl := &node.levels[i]
			if i >= len(e.levels) {
				// tail node
				nl.subtreesize -= e.len
				continue
			}

			// mid node 'before us'
			el := e.levels[i]
			nl.subtreesize += el.subtreesize - e.len
			c := el.next
			if c != nil {
				c.levels[i].prev = node
			}
			nl.next = c // when this becomes nil for levels[0], we bail
		}

		r.returnToPool(e)
		if e.id == untilId {
			return
		}
	}
}

func (r *ropeImpl[Id, T]) returnToPool(e *ropeNode[Id, T]) {
	if len(r.nodePool) == poolSize {
		return
	}

	var zero ropeLevel[Id, T]
	for i := range e.levels {
		e.levels[i] = zero
	}
	e.data = *new(T)

	r.nodePool = append(r.nodePool, e)
}

func (r *ropeImpl[Id, T]) Iter(afterId Id) iter.Seq2[Id, T] {
	// FIXME: will freak out if deletion during here

	return func(yield func(Id, T) bool) {
		e := r.byId[afterId]
		if e == nil {
			return
		}

		for {
			next := e.levels[0].next
			if next == nil {
				return
			}

			if !yield(next.id, next.data) {
				return
			}
			e = next
		}
	}
}
