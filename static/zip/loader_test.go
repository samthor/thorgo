package zip

import (
	"net/http"
	"testing"

	"github.com/samthor/thorgo/static"
)

func TestXxx(t *testing.T) {

	zl := &ZipLoader{}

	http.Handle("whatever", zl)

	var content static.Content = zl
	content.Exists("/")
}
