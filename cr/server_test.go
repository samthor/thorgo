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

	if delta, ok := cr.PerformDelete(0, 2); !ok || delta != 7 {
		t.Errorf("couldn't delete, delta=%v", delta)
	}

	s = cr.Serialize()
	if decodeString(s.Data) != "here" {
		t.Errorf("bad serialized data: %v", decodeString(s.Data))
	}
}
