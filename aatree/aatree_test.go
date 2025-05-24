package aatree

import (
	"sort"
	"testing"
)

func CompareInt(a, b int) int { return a - b }

func TestSimple(t *testing.T) {
	// Use the CompareInt function from the aatree package or define one.
	// If it's not in the same package, you'd import and use aatree.CompareInt
	// For this example, let's assume CompareInt is accessible.
	tree := New(CompareInt)

	numbersToInsert := []int{50, 51, 52, 53, 30, 20, 10, 48, 1, -100, 400, 4141}
	for _, x := range numbersToInsert {
		if !tree.Insert(x) {
			t.Errorf("failed to insert %d, Insert returned false", x)
		}
	}

	found49, _ := tree.EqualAfter(49)
	if found49 != 50 {
		t.Errorf("Find(49): expected 50, got %d", found49)
	}

	found4141, _ := tree.EqualAfter(4141)
	if found4141 != 4141 {
		t.Errorf("Find(4141): expected 4141, got %d", found4141)
	}

	found4142, _ := tree.EqualAfter(4142) // Test for a value greater than any in the tree
	if found4142 != 0 {
		t.Errorf("Find(4142): expected 0, got %d", found4142)
	}

	before4142, _ := tree.Before(4142)
	if before4142 != 4141 {
		t.Errorf("Before(4142): expected 4141, got %d", before4142)
	}

	beforeNeg100, ok := tree.Before(-100)
	if beforeNeg100 != 0 || ok {
		t.Errorf("Before(-100): expected 0, got %d", beforeNeg100)
	}

	beforeNeg99, ok := tree.Before(-99)
	if beforeNeg99 != -100 || !ok {
		t.Errorf("Before(-99): expected -100, got %d", beforeNeg99)
	}

	sortedNumbers := make([]int, len(numbersToInsert))
	copy(sortedNumbers, numbersToInsert)
	sort.Ints(sortedNumbers) // Sorts in ascending order

	for i := 1; i < len(sortedNumbers); i++ {
		currentVal := sortedNumbers[i]
		expectedPrev := sortedNumbers[i-1]
		actualPrevNode, _ := tree.Before(currentVal)

		if actualPrevNode != expectedPrev {
			t.Errorf("Before(%d): expected %d, got %d", currentVal, expectedPrev, actualPrevNode)
		}
	}

	if tree.Count() != len(numbersToInsert) {
		t.Errorf("Count(): expected %d, got %d", numbersToInsert, tree.Count())
	}

	if !tree.Remove(30) {
		t.Errorf("Remove(30), should be true")
	}
	if tree.Remove(30) {
		t.Errorf("Remove(30), should be false")
	}
	if before31, ok := tree.Before(31); before31 != 20 || !ok {
		t.Errorf("Before(31): expected 30, got %d", before31)
	}
}
