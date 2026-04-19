package asafe

import (
	"sync/atomic"
	"unsafe"
)

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

	var check K
	if less(check, check) {
		panic("zero value must not be less than itself")
	}

	return &skipImpl[K]{less: less}
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

func (s *skipEntry[K]) nextAsPointer() (p *unsafe.Pointer) {
	// this awkwardly gets the next ptr (naively converts it to local var)
	return (*unsafe.Pointer)(unsafe.Pointer(&s.next))
}

// readNext atomically returns the next entry, whether it is tombstoned, _and_ the original pointer that s.next points to (i.e., next or next+1 byte).
func (s *skipEntry[K]) readNext() (next *skipEntry[K], dead bool, actual unsafe.Pointer) {
	actual = atomic.LoadPointer(s.nextAsPointer())

	if uintptr(actual)&1 == 1 {
		// dead, move back
		// at := unsafe.Pointer(uintptr(actual) & ^uintptr(1))
		at := unsafe.Pointer(uintptr(actual) - 1)
		next = (*skipEntry[K])(at)
		dead = true
	} else {
		next = (*skipEntry[K])(actual)
	}

	return
}

func (s *skipImpl[K]) Add(k K) {
	curr := &s.head
	for {
		next, dead, actual := curr.readNext()
		_ = dead // TODO: we currently don't care if the next is alive - do we?

		// should we insert between here and next?
		// lower value is first!
		if next != nil && !s.less(k, next.value) {
			// log.Printf("%v < %v", k, next.value)
			curr = next
			continue
		}

		// alloc new entry (we assume 'actual' will be correct)
		alloc := &skipEntry[K]{next: (*byte)(actual), value: k}

		// insert atomically!
		// if not, we try whole loop again
		swapped := atomic.CompareAndSwapPointer(curr.nextAsPointer(), actual, unsafe.Pointer(alloc))
		if swapped {
			break
		}
	}
}

func (s *skipImpl[K]) Next() (k K, ok bool) {
	// curr := &s.head
	// for {

	// }

	panic("unimplemented")
}
