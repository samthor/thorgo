package queue

import (
	"context"
	"iter"
	"sync"
)

// New builds a new concurrent broadcast queue.
func New[X any]() Queue[X] {
	return &queueImpl[X]{
		subs: make(map[int]int),
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

type queueImpl[X any] struct {
	head   int
	events []X
	subs   map[int]int

	cond *sync.Cond

	observerHigh int
}

func (q *queueImpl[X]) Push(all ...X) bool {
	if len(all) == 0 {
		return false // broadcast would be wasteful
	}

	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.head += len(all)

	if len(q.subs) == 0 {
		q.events = nil
		return false // we can literally drop all, noone cares
	}

	q.events = append(q.events, all...)
	q.cond.Broadcast()

	// we have the lock again, can now check who broadcast stuff and trim events
	// if something was trimmed, we know that someone consumed us
	return q.trimEvents()
}

func (q *queueImpl[X]) Join(ctx context.Context) Listener[X] {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	who := q.observerHigh
	q.observerHigh++

	go func() {
		<-ctx.Done()

		q.cond.L.Lock()
		defer q.cond.L.Unlock()

		delete(q.subs, who)
		q.trimEvents() // we can purge events

		// wake up everyone
		// TODO: bad for large numbers of queue listeners, they all have to check if they're evicted
		q.cond.Broadcast()
	}()

	q.subs[who] = q.head

	return &queueListener[X]{q: q, who: who}
}

// trimEvents must be called under lock.
func (q *queueImpl[X]) trimEvents() bool {
	// we have the lock again, can now check who broadcast stuff and trim events
	// TODO: "slow" for large numbers of subs (O(n))
	m := q.head
	for _, cand := range q.subs {
		m = min(cand, m)
	}
	if m == q.head {
		if len(q.events) > 0 {
			q.events = nil
			return true // we always had at least one event, someone consumed it
		}
		return false
	}

	start := q.head - len(q.events)
	strip := m - start
	if strip > 0 {
		q.events = q.events[strip:]
		return true // someone consumed an event
	}
	return false
}

func (q *queueImpl[X]) wait(who int, handler func(avail []X) int) bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for {
		last, ok := q.subs[who]
		if !ok {
			// either wrong, OR we got done for
			return false
		}

		if last == q.head {
			q.cond.Wait()
			continue
		}

		start := q.head - len(q.events)
		skip := last - start
		toSend := q.events[skip:]

		consumed := handler(toSend)
		if consumed <= 0 {
			panic("must consume +ve queue entries")
		}

		consumed = min(consumed, len(toSend))
		q.subs[who] = last + consumed // move past consumed
		return true
	}
}

type queueListener[X any] struct {
	q   *queueImpl[X]
	who int
}

func (ql *queueListener[X]) Peek() (out X, ok bool) {
	q := ql.q

	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	var last int
	last, ok = q.subs[ql.who]
	if !ok {
		return
	}

	ok = last < q.head
	if !ok {
		return
	}

	start := q.head - len(q.events)
	skip := last - start
	out = q.events[skip]
	return
}

func (ql *queueListener[X]) Next() (out X, ok bool) {
	ql.q.wait(ql.who, func(avail []X) int {
		out = avail[0]
		ok = true
		return 1
	})
	return out, ok
}

func (ql *queueListener[X]) Batch() []X {
	var out []X
	ql.q.wait(ql.who, func(avail []X) int {
		out = avail
		return len(avail)
	})
	return out
}

func (ql *queueListener[X]) Iter() iter.Seq[X] {
	return func(yield func(X) bool) {
		for {
			next, ok := ql.Next()
			if !ok {
				return
			}
			if !yield(next) {
				return
			}
		}
	}
}
