package cr

import (
	"log"

	"github.com/samthor/thorgo/aatree"
)

// rangeOverConfig is the subset of rope.Rope needed to maintain range information.
type rangeOverConfig[Id any] interface {
	Between(a, b Id) (distance int, ok bool)
	Compare(a, b Id) (cmp int, ok bool)
}

// extentState contains all inner nodes to a single extent.
type extentState[Id any] struct {
	start *rangeNode[Id]
	end   *rangeNode[Id]
}

type rangeNode[Id any] struct {
	id    Id
	start bool
	state *extentState[Id]
}

type rangeOver[Id comparable] struct {
	config     rangeOverConfig[Id]
	extentTree *aatree.AATree[*rangeNode[Id]]
}

type CrRange[Id any] interface {
	Mark(a, b Id) bool
}

func NewRange[Id comparable](config rangeOverConfig[Id]) CrRange[Id] {
	compare := func(a, b *rangeNode[Id]) int {
		c, _ := config.Compare(a.id, b.id)
		return c
	}

	return &rangeOver[Id]{
		config:     config,
		extentTree: aatree.New(compare),
	}
}

func (ro *rangeOver[Id]) extentFor(at Id) *extentState[Id] {
	q := &rangeNode[Id]{id: at}

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

func (ro *rangeOver[Id]) Mark(a, b Id) bool {

	c, _ := ro.config.Compare(a, b)
	if c == 0 {
		return false // either same or invalid
	}

	if c > 0 {
		a, b = b, a // TODO: right way?
	}
	log.Printf("order of Mark: %v/%v", a, b)

	leftExtent := ro.extentFor(a)
	rightExtent := ro.extentFor(b)

	log.Printf("got left=%v %+v right=%v %+v", a, leftExtent, b, rightExtent)

	if leftExtent == nil && rightExtent == nil {

		extent := &extentState[Id]{
			start: &rangeNode[Id]{id: a, start: true},
			end:   &rangeNode[Id]{id: b},
		}
		extent.start.state = extent
		extent.end.state = extent

		ok1 := ro.extentTree.Insert(extent.start)
		ok2 := ro.extentTree.Insert(extent.end)
		if !ok1 || !ok2 {
			panic("should not already exist")
		}

		return true
	}

	panic("TODO")
}
