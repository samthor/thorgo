package rope

import (
	"fmt"
	"iter"
	"log"
	"strings"
)

const (
	bufferInitCap = 8
	poolSize      = 8
)

// New builds a new Rope[T].
func New[T any]() Rope[T] {
	out := &ropeImpl[T]{
		byId:        map[Id]*ropeNode[T]{},
		height:      1,
		rseekBuffer: make([]*ropeNode[T], 1, bufferInitCap),
		nodePool:    make([]*ropeNode[T], 0, poolSize),
	}

	out.byId[0] = &out.head
	out.head.levels = []ropeLevel[T]{
		{prev: &out.head},
	}
	return out
}

type ropeLevel[T any] struct {
	next        *ropeNode[T] // can be nil
	prev        *ropeNode[T] // always set
	subtreesize int
}

type ropeNode[T any] struct {
	id     Id
	len    int
	levels []ropeLevel[T]
	data   T
}

type ropeImpl[T any] struct {
	head   ropeNode[T]
	len    int
	lastId Id
	byId   map[Id]*ropeNode[T]
	height int // matches len(head.levels)

	rseekBuffer []*ropeNode[T]
	nodePool    []*ropeNode[T]
}

func (r *ropeImpl[T]) DebugPrint() {
	log.Printf("> rope len=%d heads=%d", r.len, r.height)

	curr := &r.head

	for {
		var parts []string

		for _, l := range curr.levels {
			key := "*"
			if l.next != nil {
				key = "+"
			}

			s := fmt.Sprintf("%s%d                  ", key, l.subtreesize)
			s = s[0:5]

			parts = append(parts, s)
		}
		log.Printf("= %v", curr.data)
		log.Printf("- %s", strings.Join(parts, "  "))

		curr = curr.levels[0].next
		if curr == nil {
			break
		}
	}

}

func (r *ropeImpl[T]) Len() int {
	return r.len
}

func (r *ropeImpl[T]) Count() int {
	return len(r.byId) - 1
}

func (r *ropeImpl[T]) Find(id Id) int {
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

func (r *ropeImpl[T]) Info(id Id) (out Info[T]) {
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

func (r *ropeImpl[T]) ByPosition(position int, biasAfter bool) (id Id, offset int) {
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

func (r *ropeImpl[T]) InsertAfter(insertAfterId Id, length int, data T) Id {
	if length < 0 {
		panic("must be +ve len")
	}

	e := r.byId[insertAfterId]
	if e == nil {
		return 0
	}

	r.lastId++
	newId := r.lastId

	var height int
	var newNode *ropeNode[T]
	var levels []ropeLevel[T]

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

		levels = make([]ropeLevel[T], height)
		newNode = &ropeNode[T]{
			data:   data,
			id:     newId,
			len:    length,
			levels: levels,
		}
	}
	r.byId[newId] = newNode

	// seek to see where it goes

	type ropeSeek[T any] struct {
		node *ropeNode[T]
		sub  int
	}
	seek := make([]ropeSeek[T], r.height)
	cseek := ropeSeek[T]{
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

			levels[i] = ropeLevel[T]{
				next:        nextI,
				prev:        n,
				subtreesize: length + nl.subtreesize - st,
			}

			nl.next = newNode
			nl.subtreesize = st

		} else {
			// this is a no-op on second go-around; we need to calc the actual insertPos for this
			// we previously gave up, `insertPos` was just the local consumed subtreesize
			for cseek.node != &r.head {
				cseek.node = cseek.node.levels[len(cseek.node.levels)-1].prev
				cseek.sub += cseek.node.levels[len(cseek.node.levels)-1].subtreesize
			}

			// ensure head has correct total height
			r.head.levels = append(r.head.levels, ropeLevel[T]{
				next:        newNode,
				prev:        &r.head,
				subtreesize: cseek.sub,
			})
			r.rseekBuffer = append(r.rseekBuffer, nil)
			r.height++

			levels[i] = ropeLevel[T]{
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

	return newId
}

func (r *ropeImpl[T]) rseekNodes(id Id) []*ropeNode[T] {
	curr := r.byId[id]
	if curr == nil {
		return nil
	}

	nodes := r.rseekBuffer
	i := 0

	for {
		ll := len(curr.levels)
		for i < ll {
			nodes[i] = curr
			i++
			if i == r.height {
				return nodes
			}
		}
		curr = curr.levels[ll-1].prev
	}
}

func (r *ropeImpl[T]) Before(a, b Id) bool {
	c, _ := r.Compare(a, b)
	return c < 0
}

func (r *ropeImpl[T]) Compare(a, b Id) (cmp int, ok bool) {
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

	mult := 1
	if len(anode.levels) < len(bnode.levels) {
		// swap more levels into anode; seek will be faster
		mult = -1
		anode, bnode = bnode, anode
	}

	curr := bnode
	anodes := r.rseekNodes(anode.id)
	if anodes == nil {
		return
	}

	// walk up the tree
	i := 1
	for {
		ll := len(curr.levels)
		for i < ll {
			// stepped "right" into the previous node tree, so it must be after us
			if curr == anodes[i] {
				return mult, true
			}
			i++
		}

		ll--
		curr = curr.levels[ll].prev
		if curr == anodes[ll] {
			// stepped "up" into the previous node tree, so must be before us
			return -mult, true
		} else if curr == &r.head {
			// stepped "up" to root, so must be after us (we never saw it in walk)
			return mult, true
		}
	}
}

func (r *ropeImpl[T]) DeleteTo(afterId, untilId Id) {
	nodes := r.rseekNodes(afterId)
	if nodes == nil {
		return
	}

	for {
		e := nodes[0].levels[0].next
		if e == nil {
			return
		}

		if len(r.nodePool) != poolSize {
			e.data = *new(T)
			r.nodePool = append(r.nodePool, e)
		}

		delete(r.byId, e.id)
		r.len -= e.len

		for i, node := range nodes {
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

		if e.id == untilId {
			return
		}
	}
}

func (r *ropeImpl[T]) Iter(afterId Id) iter.Seq[Id] {
	return func(yield func(Id) bool) {
		e := r.byId[afterId]
		if e == nil {
			return
		}

		for {
			next := e.levels[0].next
			if next == nil {
				return
			}

			if !yield(next.id) {
				return
			}
			e = next
		}
	}
}
