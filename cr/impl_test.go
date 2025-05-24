package cr

import (
	"testing"
	"unicode/utf16"
)

func encodeString(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

func flattenCr(cr ServerCr[uint16, struct{}]) string {
	out := make([]uint16, 0, cr.Len())

	for _, data := range cr.Iter() {
		out = append(out, data...)
	}

	return string(utf16.Decode(out))
}

func countCr[X comparable](cr ServerCr[uint16, X]) (count int) {
	for range cr.Iter() {
		count++
	}
	return count
}

func TestXxx(t *testing.T) {
	cr := New[uint16, struct{}]()
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

	if flattenCr(cr) != "he!!llo there" {
		t.Errorf("got unexpected: %v", flattenCr(cr))
	}
	if countCr(cr) != 4 {
		t.Errorf("expected four parts")
	}

	cr.PerformAppend(exclaimSeq, encodeString("??"), nonce)
	if flattenCr(cr) != "he!!??llo there" {
		t.Errorf("got unexpected: %v", flattenCr(cr))
	}
	if countCr(cr) != 4 {
		// merged
		t.Errorf("expected four parts (merged)")
	}
}
