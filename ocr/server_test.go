package ocr

import (
	"log"
	"reflect"
	"testing"
	"unicode/utf16"
)

func decodeString(raw []uint16) string {
	return string(utf16.Decode(raw))
}

func encodeString(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

func TestAppendZero(t *testing.T) {
	cr := New[uint16, int]()
	var ok bool

	_, ok = cr.PerformAppend(0, 2, encodeString("hello "), 1)
	if ok {
		t.Errorf("cannot insert 'over' zero id")
	}

	_, ok = cr.PerformAppend(0, 0, encodeString("hello "), 1)
	if ok {
		t.Errorf("cannot insert 'over' zero id")
	}

	_, ok = cr.PerformAppend(0, -1, encodeString("hello "), 1)
	if !ok {
		t.Errorf("can insert to -1")
	}
}

func TestServerCr(t *testing.T) {
	cr := New[uint16, int]()

	cr.PerformAppend(0, 123, encodeString("hello "), 1)
	if cr.FindAt(1) != 123-5 {
		t.Errorf("bad FindAt: wanted=%d, got=%d", 123-5, cr.FindAt(1))
	}

	cr.PerformAppend(123, 10, encodeString("there"), 1)

	if ser := cr.ReadAll(); decodeString(ser.Data) != "hello there" ||
		!reflect.DeepEqual(ser.Seq, []int{6, 123, 5, -113}) ||
		!reflect.DeepEqual(ser.Meta, []int{1, 1}) {
		t.Errorf("bad serialization: %+v", ser)
	}

	cr.PerformAppend(122, 1000, encodeString(","), 2)
	if ser := cr.ReadAll(); decodeString(ser.Data) != "hello, there" ||
		!reflect.DeepEqual(ser.Seq, []int{5, 122, 1, 878, 1, -877, 5, -113}) ||
		!reflect.DeepEqual(ser.Meta, []int{1, 2, 1, 1}) {
		t.Errorf("bad serialization: %+v", ser)
	}

	if outA, outB, ok := cr.PerformDelete(cr.FindAt(7), cr.FindAt(6)); !ok || outA != 1000 || outB != 123 {
		t.Errorf("bad delete: %v/%v", outA, outB)
	}

	if ser := cr.ReadAll(); decodeString(ser.Data) != "hellothere" {
		t.Errorf("bad serialization: %+v (%s)", ser, decodeString(ser.Data))
	}

	if cr.FindAt(5) != 122 || cr.FindAt(6) != 6 {
		t.Errorf("bad FindAt: 5=%d, 6=%d", cr.FindAt(5), cr.FindAt(6))
	}

	// move but within
	cr.PerformMove(cr.FindAt(1), cr.FindAt(10), cr.FindAt(5))
	if ser := cr.ReadAll(); decodeString(ser.Data) != "hellothere" {
		t.Errorf("bad serialization: %+v (%s)", ser, decodeString(ser.Data))
	}

	// real move
	cr.PerformMove(cr.FindAt(1), cr.FindAt(2), cr.FindAt(10))
	if ser := cr.ReadAll(); decodeString(ser.Data) != "llotherehe" {
		t.Errorf("bad serialization: %+v (%s)", ser, decodeString(ser.Data))
	}

	if cr.Len() != 10 {
		t.Errorf("bad length: %v", cr.Len())
	}
}

func TestMerge(t *testing.T) {
	cr := New[uint16, int]()

	cr.PerformAppend(0, 9, encodeString("def"), 0)
	cr.PerformAppend(0, 6, encodeString("abc"), 0)

	if ser := cr.ReadAll(); decodeString(ser.Data) != "abcdef" || !reflect.DeepEqual(ser.Seq, []int{6, 9}) {
		t.Errorf("bad serialization: %+v (%s)", ser, decodeString(ser.Data))
	}

	cr.PerformAppend(9, 3, encodeString("123"), 0)
	if ser := cr.ReadAll(); decodeString(ser.Data) != "abcdef123" || !reflect.DeepEqual(ser.Seq, []int{6, 9, 3, -6}) {
		t.Errorf("bad serialization: %+v (%s)", ser, decodeString(ser.Data))
	}

	cr.PerformMove(1, 3, 0)
	if ser := cr.ReadAll(); decodeString(ser.Data) != "123abcdef" || !reflect.DeepEqual(ser.Seq, []int{9, 9}) {
		t.Errorf("bad serialization: %+v (%s)", ser, decodeString(ser.Data))
	}

	if cr.Len() != 9 {
		t.Errorf("bad length: %v", cr.Len())
	}
}

func TestMove(t *testing.T) {
	cr := New[uint16, int]()

	cr.PerformAppend(0, 100, encodeString("hello there"), 1)
	withinDelete := cr.FindAt(7) // technically "at end" of delete

	delA, delB := cr.FindAt(5), cr.FindAt(7)
	cr.PerformDelete(delA, delB)

	if decodeString(cr.ReadAll().Data) != "hellhere" {
		t.Errorf("bad string")
	}

	// move within deleted range; should NOT become deleted (right now at least)
	a, b := cr.FindAt(1), cr.FindAt(2)
	log.Printf("searching for %d", withinDelete)
	if outA, outB, effectiveAfter, _ := cr.PerformMove(a, b, withinDelete); outA != a || outB != b || effectiveAfter != 93 {
		t.Errorf("move expected %d/%d was %d/%d after %d was %d", outA, outB, a, b, effectiveAfter, 93)
	}
	if decodeString(cr.ReadAll().Data) != "llhehere" || cr.Len() != 8 {
		t.Errorf("bad string: %v (len=%v)", decodeString(cr.ReadAll().Data), cr.Len())
	}

	pos := cr.PositionFor(93)
	if pos != 2 {
		t.Errorf("bad pos")
	}

	if outA, outB, effectiveAfter, ok := cr.PerformMove(delA, delB, 0); !ok || effectiveAfter != 0 || outA != 0 || outB != 0 {
		t.Errorf("del move should 'succeed' but be all zeros: was ok=%v %v/%v/%v", ok, outA, outB, effectiveAfter)
	}
	if decodeString(cr.ReadAll().Data) != "llhehere" || cr.Len() != 8 {
		t.Errorf("bad string: %v (len=%v)", decodeString(cr.ReadAll().Data), cr.Len())
	}
}
