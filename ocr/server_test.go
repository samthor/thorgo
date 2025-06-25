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
	if ok {
		t.Errorf("canot insert -ve data")
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
	a, b, after := cr.FindAt(1), cr.FindAt(10), cr.FindAt(5)
	if outA, outB, effectiveAfter, ok := cr.PerformMove(b, a, after); !ok || outA != a || outB != b || effectiveAfter != cr.FindAt(0) {
		t.Errorf("bad noop move")
	}
	if outA, outB, effectiveAfter, ok := cr.PerformMove(a, b, after); !ok || outA != a || outB != b || effectiveAfter != cr.FindAt(0) {
		t.Errorf("bad noop move")
	}
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
	if outId, ok := cr.ReconcileSeq(withinDelete); !ok || outId != 93 || withinDelete == 93 {
		t.Errorf("bad reconcile, wanted id=93 was=%v", outId)
	}
	if pos, _ := cr.PositionFor(93); pos != 4 {
		t.Errorf("bad position")
	}

	// move within deleted range; should NOT become deleted (right now at least)
	a, b := cr.FindAt(1), cr.FindAt(2)
	if outA, outB, effectiveAfter, _ := cr.PerformMove(a, b, withinDelete); outA != a || outB != b || effectiveAfter != 93 {
		t.Errorf("move expected %d/%d was %d/%d after %d was %d", outA, outB, a, b, effectiveAfter, 93)
	}
	if decodeString(cr.ReadAll().Data) != "llhehere" || cr.Len() != 8 {
		t.Errorf("bad string: %v (len=%v)", decodeString(cr.ReadAll().Data), cr.Len())
	}

	pos, _ := cr.PositionFor(93)
	if pos != 2 {
		t.Errorf("bad pos")
	}

	if outA, outB, effectiveAfter, ok := cr.PerformMove(delA, delB, 0); !ok || effectiveAfter != 0 || outA != 0 || outB != 0 {
		t.Errorf("del move should 'succeed' but be all zeros: was ok=%v %v/%v/%v", ok, outA, outB, effectiveAfter)
	}
	if decodeString(cr.ReadAll().Data) != "llhehere" || cr.Len() != 8 {
		t.Errorf("bad string: %v (len=%v)", decodeString(cr.ReadAll().Data), cr.Len())
	}

	if outId, ok := cr.ReconcileSeq(withinDelete); !ok || outId != 0 {
		t.Errorf("bad reconcile, wanted id=0 was=%v", outId)
	}
}

func TestDelete(t *testing.T) {
	cr := New[uint16, int]()

	cr.PerformAppend(0, 100, encodeString("hello "), 1)
	cr.PerformAppend(100, 120, encodeString("there"), 2)
	if _, _, ok := cr.PerformDelete(119, 99); !ok {
		t.Errorf("bad thingo: %v", ok)
	}

	if decodeString(cr.ReadAll().Data) != "helle" || cr.Len() != 5 {
		t.Errorf("bad string: %v (len=%v)", decodeString(cr.ReadAll().Data), cr.Len())
	}

	dels := cr.ReadDel(nil)
	if len(dels) != 2 {
		t.Errorf("bad dels")
	}

	meta := 1
	dels = cr.ReadDel(&meta)
	if !reflect.DeepEqual(dels, []SerializedStateDel[uint16, int]{
		{
			Data:  encodeString("o "),
			Meta:  1,
			Id:    100,
			After: 98,
		},
	}) {
		t.Errorf("dels wrong: %+v", dels)
	}

	meta = 2
	dels = cr.ReadDel(&meta)
	if !reflect.DeepEqual(dels, []SerializedStateDel[uint16, int]{
		{
			Data:  encodeString("ther"),
			Meta:  2,
			Id:    119,
			After: 100,
		},
	}) {
		t.Errorf("dels wrong: %+v", dels)
	}
}

func TestAppendDup(t *testing.T) {
	cr := New[uint16, int]()

	_, ok1 := cr.PerformAppend(0, 100, encodeString("hello"), 1)
	_, ok2 := cr.PerformAppend(0, 100, encodeString("llo"), 1)
	_, ok3 := cr.PerformAppend(0, 97, encodeString("he"), 1)
	if !ok1 || !ok2 || !ok3 {
		t.Errorf("should allow dup appends: %v %v %v", ok1, ok2, ok3)
	}

	if _, ok := cr.PerformAppend(0, 95, encodeString("xx"), 1); !ok {
		t.Errorf("should allow immedate before append")
	}

	if hidden, ok := cr.PerformAppend(0, 97, encodeString("xxhe"), 1); !ok || !hidden {
		t.Errorf("unexpected state: should allow but be hidden")
	}

	if _, ok := cr.PerformAppend(0, 97, encodeString("xxHe"), 1); ok {
		t.Errorf("data not same, should not allow append")
	}

	if decodeString(cr.ReadAll().Data) != "xxhello" {
		t.Errorf("bad data")
	}

	if _, ok := cr.PerformAppend(95, 600, encodeString("yy"), 2); !ok {
		t.Errorf("couldn't insert")
	}
	if decodeString(cr.ReadAll().Data) != "xxyyhello" {
		t.Errorf("bad data")
	}

	// this must pass; even though we have put "yy" in the middle, the source data 93-97 is "xxhe"
	if hidden, ok := cr.PerformAppend(0, 97, encodeString("xxhe"), 1); !ok || !hidden {
		t.Errorf("unexpected state: should allow but be hidden")
	}
	if hidden, ok := cr.PerformAppend(0, 96, encodeString("xh"), 1); !ok || !hidden {
		t.Errorf("unexpected state: should allow but be hidden")
	}
	if hidden, ok := cr.PerformAppend(0, 599, encodeString("y"), 1); !ok || !hidden {
		t.Errorf("unexpected state: should allow but be hidden")
	}

	if _, ok := cr.PerformAppend(0, 600, encodeString("yyy"), 1); ok {
		t.Errorf("should fail, data wrong")
	}

	if _, ok := cr.PerformAppend(1221512, 97, encodeString("xxhe"), 1); ok {
		t.Errorf("after ID is invalid; should not pass")
	}
}

func TestAppendDupMove(t *testing.T) {
	cr := New[uint16, int]()

	cr.PerformAppend(0, 100, encodeString("hello"), 1)
	if hidden, ok := cr.PerformAppend(0, 100, encodeString("hello"), 1); !ok || !hidden {
		t.Errorf("bad, should succeed")
	}

	cr.PerformMove(98, 99, 100)
	if decodeString(cr.ReadAll().Data) != "heoll" {
		t.Errorf("bad data")
	}

	if hidden, ok := cr.PerformAppend(0, 100, encodeString("hello"), 1); !ok || !hidden {
		t.Errorf("bad, should succeed")
	}

	if hidden, ok := cr.PerformAppend(0, 99, encodeString("ell"), 1); !ok || !hidden {
		t.Errorf("bad, should succeed")
	}
}

func TestSerialize(t *testing.T) {
	cr := New[uint16, int]()

	out := cr.ReadAll()
	if out.Data == nil || out.Seq == nil || out.Meta == nil {
		t.Error("should not have any nil values")
	}
}

func TestLen(t *testing.T) {
	cr := New[uint16, int]()

	cr.PerformAppend(0, 100, encodeString("hello"), 1)
	if cr.Len() != 5 {
		t.Error("should be two len")
	}

	cr.PerformDelete(97, 100)
	if cr.Len() != 1 {
		t.Errorf("should be one len, is: %v", cr.Len())
	}

	// doesn't add, insert in del section
	cr.PerformAppend(98, 200, encodeString("disappeared"), 2)
	if cr.Len() != 1 {
		t.Error("should be one len")
	}
}

func TestDeleteAll(t *testing.T) {
	cr := New[uint16, int]()
	cr.PerformAppend(0, 100, encodeString("hello"), 1)

	zero := cr.FindAt(0)
	low := cr.FindAt(1)
	high := cr.FindAt(cr.Len())

	log.Printf("zero=%v low=%v high=%v", zero, low, high)
}

func TestAppendRestore(t *testing.T) {
	cr := New[uint16, int]()
	cr.PerformAppend(0, 100, encodeString("hello"), 1)

	cr.PerformDelete(99, 100)

	cr.PerformRestore(99, 99)

	if decodeString(cr.ReadAll().Data) != "hell" {
		t.Errorf("should restore string, was: %v", decodeString(cr.ReadAll().Data))
	}
}

func TestRestoreTo(t *testing.T) {
	cr := New[uint16, int]()

	cr.PerformAppend(0, 100, encodeString("hello"), 1)
	cr.PerformMove(96, 97, 100)
	if decodeString(cr.ReadAll().Data) != "llohe" {
		t.Errorf("should move string, was: %v", decodeString(cr.ReadAll().Data))
	}

	cr.PerformAppend(99, 200, encodeString("!!!"), 1)
	if decodeString(cr.ReadAll().Data) != "ll!!!ohe" {
		t.Errorf("should add string, was: %v", decodeString(cr.ReadAll().Data))
	}

	if out, _ := cr.ReadSource(100, 5); decodeString(out) != "hello" {
		t.Errorf("couldn't ReadSource")
	}

	change, ok := cr.RestoreTo(100, 5)
	if !change || !ok {
		t.Errorf("change=%v ok=%v", change, ok)
	}
	if decodeString(cr.ReadAll().Data) != "hello" {
		t.Errorf("should restoreTo string, was: %v", decodeString(cr.ReadAll().Data))
	}

	cr.RestoreTo(100, 2)
	if decodeString(cr.ReadAll().Data) != "lo" {
		t.Errorf("should restoreTo string, was: %v", decodeString(cr.ReadAll().Data))
	}
	cr.RestoreTo(98, 2)

	if decodeString(cr.ReadAll().Data) != "el" {
		t.Errorf("should restoreTo string, was: %v", decodeString(cr.ReadAll().Data))
	}
}
