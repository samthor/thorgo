package cr

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

func decodeString(raw []uint16) string {
	return string(utf16.Decode(raw))
}

func TestServerCr(t *testing.T) {

	cr := New[uint16, struct{}]()
	nonce := struct{}{}

	cr.PerformAppend(0, encodeString(" there"), nonce)
	cr.PerformAppend(0, encodeString("hello"), nonce)

	var s *ServerCrState[uint16]

	s = cr.Serialize()
	if decodeString(s.Data) != "hello there" {
		t.Errorf("bad serialized data: %v", decodeString(s.Data))
	}
	if !reflect.DeepEqual(s.Seq, []int{5, 11, 6, -5}) {
		t.Errorf("bad seq: %+v", s.Seq)
	}

	if delta, ok := cr.PerformDelete(2, 2); !ok || delta != 1 {
		t.Errorf("couldn't delete, delta=%v", delta)
	}

	s = cr.Serialize()
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
	s = cr.Serialize()
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
	s = cr.Serialize()
	if decodeString(s.Data) != "hello xhere" {
		t.Errorf("bad serialized data: %v", decodeString(s.Data))
	}
	if !reflect.DeepEqual(s.Seq, []int{5, 11, 1, -10, 1, 11, 4, -6}) {
		t.Errorf("bad seq: %+v", s.Seq)
	}

}
