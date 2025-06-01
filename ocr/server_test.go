package ocr

import (
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
}
