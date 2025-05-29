package cr

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

func decodeString(raw []uint16) string {
	return string(utf16.Decode(raw))
}

func serialize(cr ServerCr[uint16, struct{}]) *ServerCrState[uint16, struct{}] {
	return cr.Read(0, cr.LastSeq())
}

func TestServerCr(t *testing.T) {

	cr := New[uint16, struct{}]()
	nonce := struct{}{}

	if zero := serialize(cr); len(zero.Data) != 0 || len(zero.Seq) != 0 {
		t.Errorf("bad zero Serialize: %+v", zero)
	}

	cr.PerformAppend(0, encodeString(" there"), nonce)
	cr.PerformAppend(0, encodeString("hello"), nonce)

	if _, _, ok := cr.PerformDelete(0, 0); ok {
		t.Errorf("cannot delete zero node")
	}

	var s *ServerCrState[uint16, struct{}]

	s = serialize(cr)
	if decodeString(s.Data) != "hello there" {
		t.Errorf("bad serialized data: %v", decodeString(s.Data))
	}
	if !reflect.DeepEqual(s.Seq, []int{5, 11, 6, -5}) {
		t.Errorf("bad seq: %+v", s.Seq)
	}

	if _, _, ok := cr.PerformDelete(2, 0); ok {
		t.Errorf("cannot delete with zero Node")
	}

	if a, b, ok := cr.PerformDelete(2, 2); !ok || a != 2 || b != 2 {
		t.Errorf("newly deleted range incorrect [%d,	%d]", a, b)
	}

	s = serialize(cr)
	if decodeString(s.Data) != "hello here" {
		t.Errorf("bad serialized data: %v", decodeString(s.Data))
	}
	if !reflect.DeepEqual(s.Seq, []int{5, 11, 1, -10, 4, 5}) {
		t.Errorf("bad seq: %+v", s.Seq)
	}

	deleted, ok := cr.PerformAppend(1, encodeString("x"), nonce)
	if deleted || !ok {
		t.Fatalf("could not append before deletion")
	}
	s = serialize(cr)
	if decodeString(s.Data) != "hello xhere" {
		t.Errorf("bad serialized data: %v", decodeString(s.Data))
	}
	if !reflect.DeepEqual(s.Seq, []int{5, 11, 1, -10, 1, 11, 4, -6}) {
		t.Errorf("bad seq: %+v", s.Seq)
	}

	deleted, ok = cr.PerformAppend(2, encodeString("X"), nonce)
	if !ok || !deleted {
		t.Errorf("expected deleted insert")
	}
	s = serialize(cr)
	if decodeString(s.Data) != "hello xhere" {
		t.Errorf("bad serialized data: %v", decodeString(s.Data))
	}
	if !reflect.DeepEqual(s.Seq, []int{5, 11, 1, -10, 1, 11, 4, -6}) {
		t.Errorf("bad seq: %+v", s.Seq)
	}

}
