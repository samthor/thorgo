package cr

import (
	"testing"
	"unicode/utf16"
)

func encodeString(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

func flattenCr[T comparable](cr *crAddImpl[uint16, T]) string {
	out := make([]uint16, 0, cr.Len())

	for _, data := range cr.Iter() {
		out = append(out, data...)
	}

	return string(utf16.Decode(out))
}

func countCr[X comparable](cr *crAddImpl[uint16, X]) (count int) {
	for range cr.Iter() {
		count++
	}
	return count
}

func TestCrAdd(t *testing.T) {
	cr := newCrAdd[uint16, struct{}]()
	nonce := struct{}{}

	cr.PerformAppend(0, encodeString(" there"), nonce)
	seq, _ := cr.PerformAppend(0, encodeString("hello"), nonce)

	if flattenCr(cr) != "hello there" {
		t.Errorf("got unexpected: %v", flattenCr(cr))
	}
	if seq != 11 {
		t.Errorf("expected 11 seq was=%v", seq)
	}
	if countCr(cr) != 2 {
		t.Errorf("expected two parts")
	}

	exclaimSeq, _ := cr.PerformAppend(8, encodeString("!!"), nonce)
	if cr.PositionFor(exclaimSeq) != 4 {
		t.Errorf("explain was not at 4, got=%d", cr.PositionFor(exclaimSeq))
	}
	if cr.PositionFor(exclaimSeq-1) != 3 {
		t.Errorf("explain was not at 3, got=%d", cr.PositionFor(exclaimSeq-1))
	}

	if flattenCr(cr) != "he!!llo there" {
		t.Errorf("got unexpected: %v", flattenCr(cr))
	}
	if countCr(cr) != 4 {
		t.Errorf("expected four parts")
	}

	questionSeq, _ := cr.PerformAppend(exclaimSeq, encodeString("??"), nonce)
	if flattenCr(cr) != "he!!??llo there" {
		t.Errorf("got unexpected: %v", flattenCr(cr))
	}
	if countCr(cr) != 4 {
		// merged
		t.Errorf("expected four parts (merged)")
	}

	if cr.PositionFor(questionSeq) != 6 {
		t.Errorf("question was not at 6, got=%d", cr.PositionFor(questionSeq))
	}
	if cr.PositionFor(questionSeq-1) != 5 {
		t.Errorf("question-1 was not at 5, got=%d", cr.PositionFor(questionSeq-1))
	}
}

func TestOps(t *testing.T) {
	cr := newCrAdd[uint16, int]()

	thereId, _ := cr.PerformAppend(0, encodeString(" there"), 100)
	helloId, _ := cr.PerformAppend(0, encodeString("hello"), 200)
	bobId, _ := cr.PerformAppend(thereId, encodeString(", bob!"), 300)

	if flattenCr(cr) != "hello there, bob!" {
		t.Error("bad cr: %V", flattenCr(cr))
	}

	if cmp, _ := cr.Compare(bobId, helloId); cmp <= 0 {
		t.Errorf("expected +ve compare, was: %v", cmp)
	}
	if distance, _ := cr.Between(helloId, bobId); distance != 12 {
		t.Errorf("expected distance, was: %v", distance)
	}

	if distance, _ := cr.Between(helloId, bobId-2); distance != 10 {
		t.Errorf("expected distance, was: %v", distance)
	}

	if distance, _ := cr.Between(bobId-3, bobId-2); distance != 1 {
		t.Errorf("expected distance, was: %v", distance)
	}
	if cmp, _ := cr.Compare(bobId-3, bobId-2); cmp <= 0 {
		t.Errorf("expected +ve compare, was: %v", cmp)
	}
}
