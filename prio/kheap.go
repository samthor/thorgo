package prio

import (
	"container/heap"
)

// NewHeap returns a new KeyQueue backed by a heap.
// It it NOT safe for use by multiple goroutines, and use must be wrapped in a lock.
func NewHeap[K comparable, P Less[P]]() (out KeyQueue[K, P]) {
	return &heapImpl[K, P]{
		at: map[K]int{},
	}
}

type kqPair[K comparable, P Less[P]] struct {
	key  K
	prio P
}

type heapImpl[K comparable, P Less[P]] struct {
	at   map[K]int
	data []kqPair[K, P]
}

// -- heap.Interface

func (kq *heapImpl[K, P]) Len() (length int) {
	return len(kq.data)
}

func (kq *heapImpl[K, P]) Less(i, j int) (less bool) {
	return kq.data[i].prio.Less(kq.data[j].prio)
}

func (kq *heapImpl[K, P]) Swap(i, j int) {
	kq.data[i], kq.data[j] = kq.data[j], kq.data[i]
	kq.at[kq.data[i].key] = i
	kq.at[kq.data[j].key] = j
}

func (kq *heapImpl[K, P]) Push(x any) {
	pair := x.(kqPair[K, P])
	kq.data = append(kq.data, pair)
	kq.at[pair.key] = len(kq.data) - 1
}

func (kq *heapImpl[K, P]) Pop() (x any) {
	pair := kq.data[len(kq.data)-1]
	kq.data = kq.data[:len(kq.data)-1]
	delete(kq.at, pair.key)
	return pair
}

// -- KeyQueue

func (kq *heapImpl[K, P]) Add(k K, p P) (anew bool) {
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

func (kq *heapImpl[K, P]) Delete(k K) (ok bool) {
	if i, ok := kq.at[k]; ok {
		heap.Remove(kq, i)
		return true
	}
	return false
}

func (kq *heapImpl[K, P]) Next() (k K, p P) {
	if len(kq.data) != 0 {
		k, p = kq.Peek()

		// we could use the return value, but we know [0] is the head
		heap.Pop(kq)
	}
	return
}

func (kq *heapImpl[K, P]) Peek() (k K, p P) {
	if len(kq.data) == 0 {
		return
	}
	pair := kq.data[0]
	return pair.key, pair.prio
}

func (kq *heapImpl[K, P]) All() (out []K, prio []P) {
	out = make([]K, kq.Len())
	prio = make([]P, kq.Len())

	for i, v := range kq.data {
		out[i] = v.key
		prio[i] = v.prio
	}

	return
}
