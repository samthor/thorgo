package bimap

// OwnerMap is a map of one owner to many subordinates.
type OwnerMap[Owner comparable, Sub comparable] struct {
	owner map[Owner]map[Sub]struct{}
	sub   map[Sub]Owner
}

func (m *OwnerMap[O, S]) init() {
	if m.owner == nil {
		m.owner = map[O]map[S]struct{}{}
		m.sub = map[S]O{}
	}
}

func (m *OwnerMap[O, S]) Size() (owners, subs int) {
	return len(m.owner), len(m.sub)
}

// Add adds the given sub to the owner.
// Fails if the sub is already owned.
func (m *OwnerMap[O, S]) Add(owner O, sub S) (ok bool) {
	m.init()

	prev, had := m.sub[sub]
	if had && prev != owner {
		return
	}

	omap := m.owner[owner]
	if omap == nil {
		omap = map[S]struct{}{}
		m.owner[owner] = omap
	}

	omap[sub] = struct{}{}
	m.sub[sub] = owner
	return true
}

func (m *OwnerMap[O, S]) Release(sub S) (owner O, ok bool) {
	if m.sub == nil {
		return
	}

	owner, ok = m.sub[sub]
	if ok {
		delete(m.sub, sub)
		delete(m.owner[owner], sub)

		if len(m.owner[owner]) == 0 {
			delete(m.owner, owner)
		}
	}

	return
}

func (m *OwnerMap[O, S]) Owner(sub S) (owner O, ok bool) {
	if m.sub != nil {
		owner, ok = m.sub[sub]
	}
	return
}

// Clear removes all ownership records for the given Owner.
// Returns the prior records.
func (m *OwnerMap[O, S]) Clear(owner O) (was []S) {
	if m.owner == nil {
		return
	}

	subs := m.owner[owner]
	if subs == nil {
		return
	}
	delete(m.owner, owner)

	was = make([]S, 0, len(subs))
	for s := range subs {
		was = append(was, s)
		delete(m.sub, s)
	}
	return
}
