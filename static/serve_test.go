package static

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type dummyContent struct {
	data map[string]string
}

func (dc *dummyContent) Exists(p string) bool {
	info, r := dc.Get(p)
	if info != nil {
		defer r.Close()
		return true
	}
	return false
}

func (dc *dummyContent) Get(p string) (*FileInfo, io.ReadCloser) {
	if dc.data == nil {
		return nil, nil
	}

	s := dc.data[p]
	if s == "" {
		return nil, nil
	}

	buf := bytes.NewBufferString(s)
	return &FileInfo{}, io.NopCloser(buf)
}

func checkBody(t *testing.T, server *httptest.Server, p, expected string) {
	r, err := http.Get(server.URL + p)
	if err != nil {
		t.Errorf("could not get path=%v: %v", p, err)
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	if string(body) != expected {
		t.Errorf("got bad reply, actual=%s expected=%s", string(body), expected)
	}
}

func TestServe(t *testing.T) {
	dummy := &dummyContent{
		data: map[string]string{},
	}
	sfs := &ServeFs{
		Content: dummy,
	}
	dummy.data["index.html"] = "Hello"

	mux := http.NewServeMux()
	mux.Handle("/", sfs)

	server := httptest.NewServer(mux)
	defer server.Close()

	checkBody(t, server, "", "Hello")
	checkBody(t, server, "/unknown", "")

	sfs.SpaMode = true
	checkBody(t, server, "/unknown", "")

	checkBody(t, server, "/unknown/hello/there", "")
	sfs.ServeNakedHtml = true
	checkBody(t, server, "/unknown/hello/there", "Hello") // only works because ServeNakedHTML

	dummy.data["unknown/hello.html"] = "Another page"
	checkBody(t, server, "/unknown/hello/there/", "Another page")

	sfs.ServeNakedHtml = false
	checkBody(t, server, "/unknown/hello/there/", "Hello")
}
