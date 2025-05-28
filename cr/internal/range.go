package internal

import (
	"github.com/samthor/thorgo/aatree"
	"github.com/samthor/thorgo/rope"
)

// rangeOverConfig is the subset of rope.Rope needed to maintain range information.
type rangeOverConfig[Id comparable] interface {
	Between(a, b Id) (distance int, ok bool)
	Compare(a, b Id) (cmp int, ok bool)
}

// extentState contains all inner nodes to a single extent.
type extentState[Id comparable] struct {
	start    *extentNode[Id]
	end      *extentNode[Id]
	internal *aatree.AATree[*rangeNode[Id]] // contains internal +ve and -ve
}

func (es *extentState[Id]) mod(id Id, by int) bool {
	search := &rangeNode[Id]{id: id}
	found, _ := es.internal.Get(search)

	if by == 0 {
		return found != nil
	}

	if found == nil {
		// re-use search and insert it
		search.delta = by
		es.internal.Insert(search)
		return true
	}

	// do mod, remove if now zero
	found.delta += by
	if found.delta == 0 {
		es.internal.Remove(found)
		return false
	}
	return true
}

type extentNode[Id comparable] struct {
	id    Id
	start bool
	state *extentState[Id]
}

type rangeOver[Id comparable] struct {
	config       rangeOverConfig[Id]
	extentTree   *aatree.AATree[*extentNode[Id]]
	rangeCompare func(a, b *rangeNode[Id]) int
	extentRope   rope.Rope[Id, *extentState[Id]]
}

type rangeNode[Id comparable] struct {
	id    Id
	delta int
}

type CrRange[Id comparable] interface {
	// Mark marks the given range.
	// Returns false if the range is zero or invalid.
	Mark(a, b Id) (newlyIncluded []Id, delta int, ok bool)

	// Release is the opposite of Mark, releasing the given range.
	Release(a, b Id) (newlyVisible []Id, delta int, ok bool)

	// ExtentCount returns the number of unique extent ranges here.
	ExtentCount() int

	// Delta returns the zero or positive delta that this range would impact if used as deletion.
	Delta() int

	// DeltaFor ...
	DeltaFor(id Id) int

	// Grow indicates that the underlying Rope has changed by this much at this node, which must be positive.
	// If this returns true, it is within a known current range and has been included.
	Grow(after Id, by int) bool
}

func NewRange[Id comparable](config rangeOverConfig[Id]) CrRange[Id] {
	extentCompare := func(a, b *extentNode[Id]) int {
		c, _ := config.Compare(a.id, b.id)
		return c
	}
	rangeCompare := func(a, b *rangeNode[Id]) int {
		c, _ := config.Compare(a.id, b.id)
		return c
	}

	return &rangeOver[Id]{
		config:       config,
		extentTree:   aatree.New(extentCompare),
		rangeCompare: rangeCompare,
		extentRope:   rope.New[Id, *extentState[Id]](),
	}
}

func (ro *rangeOver[Id]) extentFor(at Id) *extentState[Id] {
	q := &extentNode[Id]{id: at}

	before, _ := ro.extentTree.EqualBefore(q)
	if before == nil {
		return nil
	}

	if !before.start {
		if cmp, _ := ro.config.Compare(before.id, at); cmp < 0 {
			return nil
		}
	}

	after := before.state.end
	if cmp, _ := ro.config.Compare(after.id, at); cmp < 0 {
		return nil
	}

	return before.state
}

func (ro *rangeOver[Id]) Mark(a, b Id) ([]Id, int, bool) {
	c, _ := ro.config.Compare(a, b)
	if c == 0 {
		// either same, _or_ zero value (because ok is false)
		return nil, 0, false
	} else if c > 0 {
		// swap to correct order
		a, b = b, a
	}

	leftExtent := ro.extentFor(a)
	rightExtent := ro.extentFor(b)

	// we're within the same extent: short-circuit and just mod internally
	// no change to outer length or included
	if leftExtent != nil && leftExtent == rightExtent {
		leftExtent.mod(a, +1)
		leftExtent.mod(b, -1)
		return nil, 0, true
	}

	// otherwise, delete all extents within this range and re-add single combined extent

	low := a
	high := b
	toMerge := make([]*extentState[Id], 0, 2)

	// include left
	if leftExtent != nil {
		low = leftExtent.start.id
		toMerge = append(toMerge, leftExtent)
	}

	// include all in middle
	search := &extentNode[Id]{id: a}
	for {
		search, _ = ro.extentTree.After(search)
		if search == nil {
			break
		} else if cmp, _ := ro.config.Compare(search.id, b); cmp >= 0 {
			break
		}
		if search.start && search.state != rightExtent {
			toMerge = append(toMerge, search.state)
		}
	}

	// include right
	if rightExtent != nil {
		high = rightExtent.end.id
		toMerge = append(toMerge, rightExtent)
	}

	// wire up newlyIncluded (the gaps filled in)
	var newlyIncluded []Id
	if len(toMerge) == 0 {
		newlyIncluded = []Id{a, b}
	} else {
		newlyIncluded = make([]Id, 0, (len(toMerge)+1)*2)
		first := toMerge[0]
		last := toMerge[len(toMerge)-1]

		if leftExtent == nil {
			newlyIncluded = append(newlyIncluded, a, first.start.id)
		}

		for i := 1; i < len(toMerge); i++ {
			begin := toMerge[i-1].end.id
			end := toMerge[i].start.id
			newlyIncluded = append(newlyIncluded, begin, end)
		}

		if rightExtent == nil {
			newlyIncluded = append(newlyIncluded, last.end.id, b)
		}

		// piggyback to delete from rope
		prev := ro.extentRope.Info(first.end.id).Prev
		deleted := ro.extentRope.DeleteTo(prev, last.end.id)
		if deleted != len(toMerge) {
			panic("rope couldn't delete expected entries")
		}
	}

	// TODO: if toMerge is size=1, we could resize instead (but cbf'ed)
	extent := &extentState[Id]{
		start:    &extentNode[Id]{id: low, start: true},
		end:      &extentNode[Id]{id: high},
		internal: aatree.New(ro.rangeCompare),
	}
	extent.start.state = extent
	extent.end.state = extent

	var lengthDelta int

	// include data from all "here"
	for _, e := range toMerge {
		for rn := range e.internal.Iter() {
			extent.mod(rn.id, rn.delta)
		}
		ro.extentTree.Remove(e.start)
		ro.extentTree.Remove(e.end)

		delta, _ := ro.config.Between(e.start.id, e.end.id)
		lengthDelta -= delta // "restore" this range
	}

	// actually mod ourselves
	extent.mod(a, +1)
	extent.mod(b, -1)

	delta, _ := ro.config.Between(low, high)
	lengthDelta += delta // "add" this range

	// insert the new extent
	ok1 := ro.extentTree.Insert(extent.start)
	ok2 := ro.extentTree.Insert(extent.end)
	if !ok1 || !ok2 {
		panic("should not already exist")
	}

	// insert rope entry after prior extent (or root)
	lastExtent, _ := ro.extentTree.Before(extent.start)
	var insertRopeAfter Id
	if lastExtent != nil {
		insertRopeAfter = lastExtent.state.end.id
	}
	if !ro.extentRope.InsertIdAfter(insertRopeAfter, extent.end.id, delta, extent) {
		panic("can't delete marked range")
	}

	return newlyIncluded, lengthDelta, true
}

func (ro *rangeOver[Id]) ExtentCount() int {
	count := ro.extentTree.Count()
	if (count & 1) != 0 {
		panic("should not have odd count")
	}
	return count >> 1
}

// debugState returns an even number of Ids matching the extents here.
func (ro *rangeOver[Id]) debugState() []Id {
	out := make([]Id, 0, ro.extentTree.Count())

	for curr := range ro.extentTree.Iter() {
		out = append(out, curr.id)
	}

	return out
}

func (ro *rangeOver[Id]) debugWithin(id Id) []rangeNode[Id] {
	state := ro.extentFor(id)
	if state == nil {
		return nil
	}

	out := make([]rangeNode[Id], 0, state.internal.Count())
	for node := range state.internal.Iter() {
		out = append(out, *node)
	}
	return out
}

func (ro *rangeOver[Id]) Delta() int {
	return ro.extentRope.Len()
}

func (ro *rangeOver[Id]) Grow(after Id, by int) bool {
	if by <= 0 {
		panic("cannot Grow by zero or -ve data")
	}

	e := ro.extentFor(after)
	if e == nil {
		return false // not within extent
	} else if e.end.id == after {
		return false // nothing to do, at very end of extent (will be added)
	}

	// delete/add to rope with updated length
	info := ro.extentRope.Info(e.end.id)
	ro.extentRope.DeleteTo(info.Prev, info.Id)
	ro.extentRope.InsertIdAfter(info.Prev, info.Id, info.Len+by, e)

	return true
}

func (ro *rangeOver[Id]) Release(a, b Id) (newlyReleased []Id, lengthDelta int, ok bool) {
	c, _ := ro.config.Compare(a, b)
	if c == 0 {
		// either same, _or_ zero value (because ok is false)
		return nil, 0, false
	} else if c > 0 {
		// swap to correct order
		a, b = b, a
	}

	sourceExtent := ro.extentFor(a)
	if sourceExtent == nil || sourceExtent != ro.extentFor(b) {
		// invalid: cannot release what was not cleanly marked
		return
	}

	// remove from rope
	info := ro.extentRope.Info(sourceExtent.end.id)
	ro.extentRope.DeleteTo(info.Prev, info.Id)
	insertAfter := info.Prev

	// we can create 0-n new extents here
	// TODO: for now we lazily reinsert everything (gross)

	lengthDelta, _ = ro.config.Between(sourceExtent.start.id, sourceExtent.end.id)
	lengthDelta = -lengthDelta

	ro.extentTree.Remove(sourceExtent.start)
	ro.extentTree.Remove(sourceExtent.end)
	sourceExtent.mod(a, -1)
	sourceExtent.mod(b, +1)

	var active *extentState[Id]
	var count int

	lastId := sourceExtent.start.id

	for node := range sourceExtent.internal.Iter() {

		if active == nil {
			if lastId != node.id {
				newlyReleased = append(newlyReleased, lastId, node.id)
			}

			if node.delta <= 0 || count != 0 {
				panic("bad delta for start")
			}

			active = &extentState[Id]{
				internal: aatree.New(ro.rangeCompare),
				start: &extentNode[Id]{
					id:    node.id,
					start: true,
				},
				end: &extentNode[Id]{
					// we don't know its final ID yet
				},
			}
			active.start.state = active
			active.end.state = active
		}

		active.mod(node.id, node.delta)
		count += node.delta

		if count > 0 {
			continue // not finished yet
		}

		active.end.id = node.id
		ro.extentTree.Insert(active.start)
		ro.extentTree.Insert(active.end)

		delta, _ := ro.config.Between(active.start.id, active.end.id)
		lengthDelta += delta

		ro.extentRope.InsertIdAfter(insertAfter, active.end.id, delta, active)
		insertAfter = active.end.id

		active = nil
		lastId = node.id
	}

	if lastId != sourceExtent.end.id {
		newlyReleased = append(newlyReleased, lastId, sourceExtent.end.id)
	}

	if active != nil || count != 0 {
		panic("no node closure")
	} else if lengthDelta > 0 {
		panic("lengthDelta must be -ve or 0")
	}

	return newlyReleased, lengthDelta, true
}

func (ro *rangeOver[Id]) DeltaFor(id Id) int {
	var innerDelta int

	left, _ := ro.extentTree.Before(&extentNode[Id]{id: id})

	if left != nil && left.start {
		// include the within part of this entry
		innerDelta, _ = ro.config.Between(left.state.start.id, id)
		left, _ = ro.extentTree.Before(left)
	}

	if left == nil {
		return innerDelta
	}

	var zeroId Id
	delta, _ := ro.extentRope.Between(zeroId, left.state.end.id)
	return delta + innerDelta
}
