package static

import (
	"testing"
)

func TestGetFileHash(t *testing.T) {
	hash := GetFileHash("/assets/index-_ZBaMDvt.js")
	if hash != "_ZBaMDvt" {
		t.Errorf("bad hash: %v", hash)
	}
}
