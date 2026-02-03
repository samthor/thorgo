package transport

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestJSON(t *testing.T) {
	raw := `"hello"1`

	var v any
	dec := json.NewDecoder(bytes.NewBuffer([]byte(raw)))
	err := dec.Decode(&v)
	t.Logf("got v=%v err=%v", v, err)

	err = dec.Decode(&v)
	t.Logf("got whatever=%v err=%v", v, err)
}
