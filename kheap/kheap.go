package kheap

import (
	"container/heap"
)

type Less[X Less[X]] interface {
	// Less checks if this item is before the passed item.
	//
	// This attaches to the generic, but has the same meaning as the sort package.
	Less(other X) (is bool)
}

type KeyQueue[K comparable, P Less[P]] interface {
	// Add inserts this K with the given priority P.
	// If the K already exists, this re-orders it if the priority has changed.
	Add(k K, p P) (anew bool)

	// PopFront removes an item from the front of this KeyQueue.
	// It retuns the zero values if this is empty.
	PopFront() (k K, p P)

	// PopFront removes an item from the back of this KeyQueue.
	// It retuns the zero values if this is empty.
	PopBack() (k K, p P)

	// Len returns the number of items here.
	Len() (length int)

	// All copies the data into an output slice.
	// This is not fast, and is for testing.
	All() (out []K, prio []P)
}

type kqPair[K comparable, P Less[P]] struct {
	key  K
	prio P
}

type kqImpl[K comparable, P Less[P]] struct {
	at   map[K]int
	data []kqPair[K, P]
}

func New[K comparable, P Less[P]]() (out KeyQueue[K, P]) {
	return &kqImpl[K, P]{
		at: map[K]int{},
	}
}

// -- heap.Interface

func (kq *kqImpl[K, P]) Len() (length int) {
	return len(kq.data)
}

func (kq *kqImpl[K, P]) Less(i, j int) (less bool) {
	return kq.data[i].prio.Less(kq.data[j].prio)
}

func (kq *kqImpl[K, P]) Swap(i, j int) {
	kq.data[i], kq.data[j] = kq.data[j], kq.data[i]
	kq.at[kq.data[i].key] = i
	kq.at[kq.data[j].key] = j
}

func (kq *kqImpl[K, P]) Push(x any) {
	pair := x.(kqPair[K, P])
	kq.data = append(kq.data, pair)
	kq.at[pair.key] = len(kq.data) - 1
}

func (kq *kqImpl[K, P]) Pop() (x any) {
	pair := kq.data[len(kq.data)-1]
	kq.data = kq.data[:len(kq.data)-1]
	delete(kq.at, pair.key)
	return pair
}

// -- KeyQueue

func (kq *kqImpl[K, P]) Add(k K, p P) (anew bool) {
	if i, ok := kq.at[k]; ok {
		existing := kq.data[i].prio
		if !existing.Less(p) && !p.Less(existing) {
			return false // nothing to do
		}

		kq.data[i].prio = p
		heap.Fix(kq, i)
		return false
	}

	heap.Push(kq, kqPair[K, P]{k, p})
	return true
}

func (kq *kqImpl[K, P]) PopFront() (k K, p P) {
	if len(kq.data) == 0 {
		return
	}

	pair := kq.data[0]
	heap.Pop(kq) // internally modifies slice

	return pair.key, pair.prio
}

func (kq *kqImpl[K, P]) PopBack() (k K, p P) {
	length := len(kq.data)
	if length == 0 {
		return
	}
	at := length - 1
	pair := kq.data[at]
	heap.Remove(kq, at) // internally modifies slice

	return pair.key, pair.prio
}

func (kq *kqImpl[K, P]) All() (out []K, prio []P) {
	out = make([]K, kq.Len())
	prio = make([]P, kq.Len())

	for i, v := range kq.data {
		out[i] = v.key
		prio[i] = v.prio
	}

	return
}
