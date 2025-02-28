package sse

import (
	"bytes"
	"testing"
)

func TestComment(t *testing.T) {
	b := bytes.NewBuffer([]byte{})

	m := Message{
		Comment: "Hey\nNerds\nWhat",
	}
	expected := ": Hey\n: Nerds\n: What\n\n"

	count, err := m.WriteTo(b)
	if err != nil {
		t.Errorf("got non-nil write err: %v", err)
	}
	if count != int64(len(expected)) {
		t.Errorf("expected %d bytes, was=%v", len(expected), count)
	}

	out := b.String()
	if out != expected {
		t.Errorf("got invalid out: was=%v expected=%v", out, expected)
	}
}

func TestJSON(t *testing.T) {
	b := bytes.NewBuffer([]byte{})

	type testObj struct {
		X int64
		Y string
	}
	test := testObj{X: 123}

	m := Message{
		Data: test,
		JSON: true,
	}
	Write(b, m)

	if expected := "data: {\"X\":123,\"Y\":\"\"}\n\n"; b.String() != expected {
		t.Errorf("bad JSON, got: %v wanted %v", b.String(), expected)
	}
}
