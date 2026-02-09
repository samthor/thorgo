package queue

import (
	"sync"
)

// New builds a new concurrent broadcast queue.
func New[X any]() (q Queue[X]) {
	return &queueImpl[X]{
		subs: make(map[int]int),
		cond: sync.NewCond(&sync.Mutex{}),
	}
}
