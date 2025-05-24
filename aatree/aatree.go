package aatree

type treeNode[X any] struct {
	level int
	left  *treeNode[X]
	right *treeNode[X]
	data  X
}

// CompareFunc is a function type that compares two elements of type X.
// It should return:
//   - a negative integer if a < b
//   - zero if a == b
//   - a positive integer if a > b
type CompareFunc[X any] func(a, b X) int

// AATree represents an AA tree structure.
// The generic type X is the type of data stored in the tree.
type AATree[X any] struct {
	root    *treeNode[X]
	count   int
	change  bool
	compare CompareFunc[X]
}

// New creates a new, empty AA tree with the given comparison function.
func New[X any](compare CompareFunc[X]) *AATree[X] {
	return &AATree[X]{
		compare: compare,
	}
}

// Clear removes all elements from the tree.
func (t *AATree[X]) Clear() {
	t.root = nil
	t.count = 0
}

// Count returns the number of items in this tree.
func (t *AATree[X]) Count() int {
	return t.count
}

// Has checks if this tree contains the given data, based on the compare function.
func (t *AATree[X]) Has(data X) bool {
	node := t.root

	for node != nil {
		c := t.compare(data, node.data)
		if c < 0 {
			node = node.left
		} else if c > 0 {
			node = node.right
		} else {
			return true
		}
	}
	return false
}

// EqualBefore finds the node with the given data or the node closest before the passed argument.
// It returns a pointer to the data, or nil if not found or tree is empty.
func (t *AATree[X]) EqualBefore(data X) (X, bool) {
	return t.equalBefore(t.root, data)
}

// Before finds the node immediately before the passed data.
// It returns a pointer to the data, or nil if not found or tree is empty.
func (t *AATree[X]) Before(data X) (X, bool) {
	return t.before(t.root, data)
}

// EqualAfter finds the node with the given data or the node closest above the passed argument.
// It returns a pointer to the data, or nil if not found or tree is empty.
func (t *AATree[X]) EqualAfter(data X) (X, bool) {
	return t.equalAfter(t.root, data)
}

// After finds the node immediately after the passed data.
// It returns a pointer to the data, or nil if not found or tree is empty.
func (t *AATree[X]) After(data X) (X, bool) {
	return t.after(t.root, data)
}

// Insert inserts the value into the tree.
// It updates the previous value if the compare function returns zero.
// Returns true if a new node was inserted.
// This returns false if the node already existed, however, the data might still have changed (it just compared as equal).
func (t *AATree[X]) Insert(data X) bool {
	t.change = false
	t.root = t.insert(t.root, data)
	return t.change
}

// Remove removes the value from the tree.
// Returns true if there was a change (node was removed), false otherwise.
func (t *AATree[X]) Remove(data X) bool {
	t.change = false
	t.root = t.remove(t.root, data)
	return t.change
}

func (t *AATree[X]) skew(node *treeNode[X]) *treeNode[X] {
	if node.left == nil {
		return node
	}
	if node.left.level != node.level {
		return node
	}
	leftNode := node.left
	node.left = leftNode.right
	leftNode.right = node
	return leftNode
}

func (t *AATree[X]) split(node *treeNode[X]) *treeNode[X] {
	if node.right == nil || node.right.right == nil {
		return node
	}
	if node.right.right.level != node.level {
		return node
	}
	rightNode := node.right
	node.right = rightNode.left
	rightNode.left = node
	rightNode.level++
	return rightNode
}

func (t *AATree[X]) equalBefore(node *treeNode[X], data X) (x X, out bool) {
	for node != nil {
		c := t.compare(data, node.data)
		if c < 0 {
			node = node.left
			continue
		}

		if c != 0 {
			// recursive on left so we can compare value
			within, ok := t.equalBefore(node.left, data)
			if ok {
				return within, true
			}
		}
		return node.data, true
	}
	return
}

func (t *AATree[X]) before(node *treeNode[X], data X) (x X, ok bool) {
	for node != nil {
		c := t.compare(data, node.data)
		if c > 0 {
			// current node.data is less than data, so it's a candidate
			x = node.data
			ok = true
			node = node.right // try to find a larger candidate (closer to data)
		} else {
			// current node.data is >= data, so it's not before data
			// move to the left to find smaller values
			node = node.left
		}
	}
	return
}

func (t *AATree[X]) equalAfter(node *treeNode[X], data X) (x X, out bool) {
	for node != nil {
		c := t.compare(data, node.data)
		if c > 0 {
			node = node.right
			continue
		}

		if c != 0 {
			// recursive on left so we can compare value
			within, ok := t.equalAfter(node.left, data)
			if ok {
				return within, true
			}
		}
		return node.data, true
	}
	return
}

func (t *AATree[X]) after(node *treeNode[X], data X) (x X, ok bool) {
	for node != nil {
		c := t.compare(data, node.data)
		if c < 0 {
			// current node.data is more than data, so it's a candidate
			x = node.data
			ok = true
			node = node.left // try to find a smaller candidate (closer to data)
		} else {
			// current node.data is <= data, so it's not before data
			// move to the right to find smaller values
			node = node.right
		}
	}
	return
}

func (t *AATree[X]) insert(node *treeNode[X], data X) *treeNode[X] {
	if node == nil {
		t.count++
		t.change = true
		return &treeNode[X]{
			level: 1,
			data:  data,
		}
	}

	c := t.compare(data, node.data)

	if c < 0 {
		node.left = t.insert(node.left, data)
	} else if c > 0 {
		node.right = t.insert(node.right, data)
	} else {
		node.data = data
		return node // found ourselves
	}

	node = t.skew(node)
	node = t.split(node)
	return node
}

func (t *AATree[X]) remove(node *treeNode[X], data X) *treeNode[X] {
	if node == nil {
		return nil
	}
	c := t.compare(data, node.data)

	if c < 0 {
		node.left = t.remove(node.left, data)
	} else if c > 0 {
		node.right = t.remove(node.right, data)
	} else {
		t.count--
		t.change = true

		if node.left == nil && node.right == nil {
			return nil
		} else if node.left == nil {
			return node.right
		} else if node.right == nil {
			return node.left
		}

		successor := findMinNode(node.right)
		node.data = successor.data
		node.right = t.remove(node.right, successor.data)
	}

	// Rebalance
	var leftLevel, rightLevel int
	if node.left != nil {
		leftLevel = node.left.level
	}
	if node.right != nil {
		rightLevel = node.right.level
	}

	newLevel := min(leftLevel, rightLevel) + 1
	if newLevel < node.level {
		node.level = newLevel
		if node.right != nil && newLevel < node.right.level {
			node.right.level = newLevel
		}
	}

	node = t.skew(node)
	node = t.split(node)

	if node.right != nil {
		node.right = t.skew(node.right)
		node.right = t.split(node.right)
		if node.right.right != nil {
			node.right.right = t.split(node.right.right)
		}
	}

	return node
}

// findMinNode finds the node with the minimum value in the subtree rooted at `node`.
// Assumes `node` is not nil.
func findMinNode[X any](node *treeNode[X]) *treeNode[X] {
	for node.left != nil {
		node = node.left
	}
	return node
}
