package bimap

// Map is a simple BiMap.
// It can be inverted to get the other side.
type Map[A, B comparable] struct {
	fwd map[A]B
	rwd map[B]A
}

func (m *Map[A, B]) init() {
	if m.fwd == nil {
		m.fwd = map[A]B{}
		m.rwd = map[B]A{}
	}
}

func (m *Map[A, B]) Len() (length int) {
	return len(m.fwd)
}

func (m *Map[A, B]) Put(a A, b B) (ok bool) {
	m.init()

	if _, exists := m.fwd[a]; exists {
		return
	}
	if _, exists := m.rwd[b]; exists {
		return
	}

	m.fwd[a] = b
	m.rwd[b] = a
	return true
}

func (m *Map[A, B]) Delete(a A) (change bool) {
	m.init()

	prev, exists := m.fwd[a]
	if exists {
		delete(m.fwd, a)
		delete(m.rwd, prev)
		return true
	}

	return false
}

func (m *Map[A, B]) DeleteFar(b B) (change bool) {
	m.init()

	prev, exists := m.rwd[b]
	if exists {
		delete(m.fwd, prev)
		delete(m.rwd, b)
		return true
	}

	return false
}

func (m *Map[A, B]) Get(a A) (b B, has bool) {
	m.init()

	b, has = m.fwd[a]
	return
}

func (m *Map[A, B]) GetFar(b B) (a A, has bool) {
	m.init()

	a, has = m.rwd[b]
	return
}

func (m *Map[A, B]) Invert() (out *Map[B, A]) {
	m.init()

	return &Map[B, A]{
		fwd: m.rwd,
		rwd: m.fwd,
	}
}
