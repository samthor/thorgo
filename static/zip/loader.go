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

// ZipLoader allows serving website content from a remote zip file.
type ZipLoader struct {
	Url   string
	Local string

	cache *CacheState

	lock sync.RWMutex
}

type cacheEntry struct {
	b    []byte
	info static.FileInfo
}

type CacheState struct {
	Map  map[string]cacheEntry
	When time.Time
}

func buildCache(p string) (*CacheState, error) {
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

	return &CacheState{
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

func (zl *ZipLoader) Update(ctx context.Context) error {
	log.Printf("getting zip from: %s", zl.Url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zl.Url, nil)
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("unexpected status for %s: %d", zl.Url, res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if ct != "application/zip" {
		return fmt.Errorf("unexpected Content-Type for %s: %s", zl.Url, ct)
	}
	log.Printf("got zip from remote: %s", zl.Url)

	writeTo := fmt.Sprintf("%s.tmp-%d", zl.Local, rand.Int())
	defer os.Remove(writeTo)

	f, err := os.OpenFile(writeTo, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, res.Body)
	if err != nil {
		return err
	}

	log.Printf("building cache from local: %s", writeTo)

	newCache, err := buildCache(writeTo)
	if err != nil {
		return err
	}

	// rename file under lock
	zl.lock.Lock()
	defer zl.lock.Unlock()

	log.Printf("moving zip into location: %s", zl.Local)

	err = os.Rename(writeTo, zl.Local)
	if err != nil {
		return err
	}
	zl.cache = newCache

	log.Printf("done, cache updated :thumbsup:")
	return nil
}
