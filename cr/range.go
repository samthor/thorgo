package cr

import (
	"github.com/samthor/thorgo/aatree"
)

// rangeOverConfig is the subset of rope.Rope needed to maintain range information.
type rangeOverConfig[Id any] interface {
	Between(a, b Id) (distance int, ok bool)
	Compare(a, b Id) (cmp int, ok bool)
}

type rangeNode[Id any] struct {
	id      Id
	count   int  // delta of ranges here
	isStart bool // is this the start of unified range
	isEnd   bool // is this the end of unified range
}

type rangeOver[Id any] struct {
	config     rangeOverConfig[Id]
	rangeTree  *aatree.AATree[*rangeNode[Id]]
	extentTree *aatree.AATree[*rangeNode[Id]]
}

func newRange[Id any](config rangeOverConfig[Id]) *rangeOver[Id] {
	compare := func(a, b *rangeNode[Id]) int {
		c, _ := config.Compare(a.id, b.id)
		return c
	}

	return &rangeOver[Id]{
		config:     config,
		rangeTree:  aatree.New(compare), // contains all nodes
		extentTree: aatree.New(compare), // only contains isStart/isEnd nodes
	}
}

func (ro *rangeOver[Id]) Insert(a, b Id) bool {

	c, _ := ro.config.Compare(a, b)
	if c == 0 {
		return false // either same or invalid
	}

	if c < 0 {
		a, b = b, a // TODO: right way?
	}

	if ro.extentTree.Count() == 0 {

		left := &rangeNode[Id]{
			id:      a,
			count:   1,
			isStart: true,
		}
		right := &rangeNode[Id]{
			id:    b,
			count: 1,
			isEnd: true,
		}

		ro.extentTree.Insert(left)
		ro.extentTree.Insert(right)
		ro.rangeTree.Insert(left)
		ro.rangeTree.Insert(right)

		return true
	}

	panic("TODO: anything bar a single entry lol")
}
