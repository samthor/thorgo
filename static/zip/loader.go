package zip

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/samthor/thorgo/static"
)

var (
	reFileHash = regexp.MustCompile(`(-|\.)([a-z0-9A-Z]{6,24})\.`)
)

func hashForFile(name string) string {
	out := reFileHash.FindStringSubmatch(name)
	if out == nil {
		return ""
	}
	return out[2]
}

// ZipLoader allows serving website content from a local zip file.
// Local should be set as the local filename.
type ZipLoader struct {
	Local string
	lock  sync.RWMutex
	cache *cacheState
}

// ServeHTTP allows uploading a zip file directly. This just accepts the file blindly.
func (zl *ZipLoader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	err := zl.Update(r.Body, time.Time{})
	if err != nil {
		fmt.Fprintf(w, "can't upload: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fmt.Fprintf(w, "ok, uploaded")
}

type cacheEntry struct {
	b    []byte
	info static.FileInfo
}

type cacheState struct {
	Map  map[string]cacheEntry
	When time.Time
}

func buildCache(p string) (*cacheState, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	r, err := zip.NewReader(f, stat.Size())
	if err != nil {
		return nil, err
	}

	out := make(map[string]cacheEntry)
	for _, file := range r.File {
		if file.FileInfo().IsDir() {
			continue // zip stores dir entry, don't care
		}

		zh, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer zh.Close()

		all, err := io.ReadAll(zh)
		if err != nil {
			return nil, err
		}

		var info static.FileInfo

		// find optional hash in filename
		info.Hash = hashForFile(file.Name)
		if info.Hash == "" {
			info.ContentHash = true
			if file.CRC32 != 0 {
				info.Hash = fmt.Sprintf("%08x", file.CRC32)
			}
		}

		out[file.Name] = cacheEntry{
			b:    all,
			info: info,
		}
	}

	return &cacheState{
		When: stat.ModTime(),
		Map:  out,
	}, nil
}

func (zl *ZipLoader) Get(path string) (*static.FileInfo, io.ReadCloser) {
	zl.lock.RLock()
	defer zl.lock.RUnlock()

	if zl.cache == nil {
		return nil, nil
	}

	ce, ok := zl.cache.Map[path]
	if !ok {
		return nil, nil
	}

	rc := io.NopCloser(bytes.NewBuffer(ce.b))
	return &ce.info, rc
}

func (zl *ZipLoader) Exists(path string) bool {
	zl.lock.RLock()
	defer zl.lock.RUnlock()

	if zl.cache == nil {
		return false
	}
	_, ok := zl.cache.Map[path]
	return ok
}

// Load sets up content from the local zip file.
func (zl *ZipLoader) Load() (exists bool, err error) {
	zl.lock.Lock()
	defer zl.lock.Unlock()

	newCache, err := buildCache(zl.Local)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	zl.cache = newCache
	return true, nil
}

// When indicates when the zip file was loaded from cache.
func (zl *ZipLoader) When() time.Time {
	zl.lock.RLock()
	defer zl.lock.RUnlock()
	if zl.cache != nil {
		return zl.cache.When
	}
	return time.Time{}
}

func (zl *ZipLoader) Count() int {
	zl.lock.RLock()
	defer zl.lock.RUnlock()
	return len(zl.cache.Map)
}

// Update puts a zip from the given reader over the prior cached content.
func (zl *ZipLoader) Update(r io.Reader, mtime time.Time) error {
	writeTo := fmt.Sprintf("%s.tmp-%d", zl.Local, rand.Int())
	defer os.Remove(writeTo)

	f, err := os.OpenFile(writeTo, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	f.Close() // we move this by filename later
	if err != nil {
		return err
	}

	// don't check err for this
	if !mtime.IsZero() {
		os.Chtimes(writeTo, time.Time{}, mtime.In(time.UTC))
	}

	// create cache first and THEN swap into place
	log.Printf("building cache from tmpfile: %s", writeTo)
	newCache, err := buildCache(writeTo)
	if err != nil {
		return err
	}

	// rename file under lock
	zl.lock.Lock()
	defer zl.lock.Unlock()

	log.Printf("moving zip into location: %s => %s", writeTo, zl.Local)

	err = os.Rename(writeTo, zl.Local)
	if err != nil {
		return err
	}
	zl.cache = newCache
	return nil
}

// Fetch fetches the zip at the given URL and swaps it in-place, replacing the current file.
func (zl *ZipLoader) Fetch(ctx context.Context, url string) error {
	log.Printf("getting zip from: %s", url)

	var mtime time.Time
	stat, _ := os.Stat(zl.Local)
	if stat != nil {
		mtime = stat.ModTime()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if !mtime.IsZero() {
		// If-Modified-Since must be in "GMT" even though this is UTC
		req.Header.Set("If-Modified-Since", mtime.In(time.FixedZone("GMT", 0)).Format(time.RFC1123))
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		log.Printf("got HTTP 304, skipping update")
		return nil
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status for %s: %d", url, res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if ct != "application/zip" {
		return fmt.Errorf("unexpected Content-Type for %s: %s", url, ct)
	}
	log.Printf("got zip from remote: %s", url)

	lastModified, _ := time.ParseInLocation(time.RFC1123, res.Header.Get("Last-Modified"), time.FixedZone("GMT", 0))
	if !lastModified.IsZero() {
		log.Printf("got last-modified from remote: %v", lastModified)
	}

	return zl.Update(res.Body, lastModified)
}
