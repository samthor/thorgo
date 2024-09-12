package zip

import (
	"context"
	"os"
	"testing"
)

func TestXxx(t *testing.T) {
	os.Remove("./test.zip")

	zl := &ZipLoader{
		Local: "./test.zip",
	}

	var err error

	err = zl.Fetch(context.Background(), "https://storage.googleapis.com/hwhistlr.appspot.com/sites/locksrv.zip")
	if err != nil {
		t.Errorf("couldn't fetch locksrv: %v", err)
	}

	err = zl.Fetch(context.Background(), "https://storage.googleapis.com/hwhistlr.appspot.com/sites/locksrv.zip")
	if err != nil {
		t.Errorf("couldn't fetch locksrv: %v", err)
	}
}
