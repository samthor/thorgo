package asafe

import (
	"log"
	"sync/atomic"
	"unsafe"
)

func init() {
	// since "dead" is at the start, we assume it's fixed (before K)
	var x skipEntry[any]
	deadOffset := unsafe.Offsetof(x.dead)
	if deadOffset != 1 {
		panic("bad deadOffset")
	}
}

type SkipQueue[K any] interface {
	// All copies everything here, for testing.
	All() (out []K)

	// Add adds this K to the SkipQueue.
	// The K can be duplicated.
	Add(k K)

	// Next returns the next K here, or the zero value and false if empty.
	Next() (k K, ok bool)
}

// NewSkipQueue creates a concurrent-safe SkipQueue.
func NewSkipQueue[K any](less func(a, b K) (is bool)) (out SkipQueue[K]) {
	if less == nil {
		panic("must provide less()")
	}
	impl := &skipImpl[K]{
		less: less,
	}

	return impl
}

type skipEntry[K any] struct {
	alive, dead byte
	next        *byte // points to the next's alive or dead (or nil)
	value       K
}

type skipImpl[K any] struct {
	less func(a, b K) (is bool)
	head skipEntry[K]
}

func (s *skipImpl[K]) All() (out []K) {
	curr := &s.head

	for {
		next, dead, _ := curr.readNext()
		if next == nil {
			break
		} else if dead {
			continue
		}

		out = append(out, next.value)
		curr = next
	}

	return out
}

// readNext returns the next entry, whether it is tombstoned, _and_ the original pointer that s.next points to (i.e., next or next+1 byte).
func (s *skipEntry[K]) readNext() (next *skipEntry[K], dead bool, actual unsafe.Pointer) {
	ptr := unsafe.Pointer(s.next) // just a type cast; is "same"
	actual = atomic.LoadPointer(&ptr)

	var nextPtr unsafe.Pointer
	if uintptr(actual)&1 == 1 {
		// dead, move back
		nextPtr = unsafe.Pointer(uintptr(actual) - 1)
		dead = true
	} else {
		nextPtr = actual
	}

	next = (*skipEntry[K])(nextPtr)
	return
}

func (s *skipImpl[K]) Add(k K) {
	var failures int

	curr := &s.head
	for {
		next, dead, actual := curr.readNext()
		_ = dead // TODO: we currently don't care if the next is alive - do we?

		// should we insert between here and next?
		// lower value is first!
		if next != nil && !s.less(k, next.value) {
			curr = next
			continue
		}

		// alloc new entry (we assume 'actual' will be correct)
		alloc := &skipEntry[K]{next: (*byte)(actual), value: k}

		// insert atomically!
		// if not, we try whole loop again
		ptr := unsafe.Pointer(curr.next)
		swapped := atomic.CompareAndSwapPointer(&ptr, actual, unsafe.Pointer(alloc))
		if !swapped {
			failures++
			if failures >= 5 {
				panic("too many failures")
			}
			log.Printf("!!! CAS failed")
			continue // try whole loop again
		}

		// curr.next = &alloc.alive
		break
	}
}

func (s *skipImpl[K]) Next() (k K, ok bool) {
	// curr := &s.head
	// for {

	// }

	panic("unimplemented")
}
