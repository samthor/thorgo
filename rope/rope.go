package rope

import (
	"fmt"
	"iter"
	"log"
	"strings"
)

// New builds a new Rope[T].
func New[T any]() Rope[T] {
	out := &ropeImpl[T]{
		byId:   map[Id]*ropeNode[T]{},
		height: 1,
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
	len    int
	lastId Id
	byId   map[Id]*ropeNode[T]
	head   ropeNode[T]
	height int // matches len(head.levels)
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
	height := randomHeight()

	levels := make([]ropeLevel[T], height)
	for i := range levels {
		levels[i].prev = &r.head
	}
	newNode := &ropeNode[T]{
		data:   data,
		id:     newId,
		len:    length,
		levels: levels,
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

	nodes := make([]*ropeNode[T], r.height)
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

// stepsTo counts the steps from the 'deeper' node to the lower node.
// This is dangerous with unknown arguments because it might just run forever.
func stepsTo[T any](from, to *ropeNode[T], depth int) (count int) {
	for from != to {
		count++
		from = from.levels[depth].prev
	}
	return
}

func (r *ropeImpl[T]) Before(a, b Id) bool {
	c, _ := r.Compare(a, b)
	return c < 0
}

func (r *ropeImpl[T]) Compare(a, b Id) (cmp int, ok bool) {
	if a == b {
		_, ok = r.byId[a]
		return 0, ok
	}

	anodes := r.rseekNodes(a)
	if anodes == nil {
		return
	}
	// TODO: this could be faster because the second rseek could just step until a match
	bnodes := r.rseekNodes(b)
	if bnodes == nil {
		return
	}

	target := &r.head
	i := r.height - 1

	for i >= 0 && anodes[i] == bnodes[i] {
		target = anodes[i]
		i--
	}

	astep := stepsTo(anodes[i], target, i)
	bstep := stepsTo(bnodes[i], target, i)
	return astep - bstep, true
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
