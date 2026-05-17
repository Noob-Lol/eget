package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestDownloadArchiveUsesCompleteCacheWhenMetaMatches(t *testing.T) {
	cacheDir := t.TempDir()
	req := DownloadRequest{
		URL:      "https://example.com/go.tar.gz",
		CacheDir: cacheDir,
		SDK:      "go",
		Version:  "1.21.1",
		Filename: "go.tar.gz",
	}
	finalPath := sdkDownloadFinalPath(req)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(finalPath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write complete cache: %v", err)
	}
	if err := saveDownloadMeta(sdkDownloadMetaPath(req), downloadMeta{
		Schema:   1,
		URL:      req.URL,
		Filename: req.Filename,
		Size:     int64(len("archive")),
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.Eq(t, finalPath, result.Path)
	assert.True(t, result.FromCache)
	assert.Eq(t, int64(len("archive")), result.Size)
}

func TestDownloadArchiveResumesPartWhenMetaMatches(t *testing.T) {
	body := []byte("hello resumed archive")
	var rangeHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		switch r.Method {
		case http.MethodHead:
			return
		case http.MethodGet:
			rangeHeader = r.Header.Get("Range")
			if rangeHeader != "bytes=6-" {
				t.Fatalf("unexpected range header %q", rangeHeader)
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)-6))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 6-%d/%d", len(body)-1, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[6:])
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	req := DownloadRequest{URL: server.URL + "/go.tar.gz", CacheDir: t.TempDir(), SDK: "go", Version: "1.21.1", Filename: "go.tar.gz"}
	partPath := sdkDownloadPartPath(req)
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(partPath, body[:6], 0o644); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := saveDownloadMeta(sdkDownloadMetaPath(req), downloadMeta{
		Schema:   1,
		URL:      req.URL,
		Filename: req.Filename,
		Size:     int64(len(body)),
		ETag:     `"v1"`,
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.Eq(t, "bytes=6-", rangeHeader)
	assert.True(t, result.Resumed)
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	assert.Eq(t, string(body), string(data))
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Fatalf("expected part file to be renamed, stat err=%v", err)
	}
}

func TestDownloadArchiveRestartsWhenETagChanges(t *testing.T) {
	body := []byte("new archive")
	var sawRange bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"v2"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		if r.Header.Get("Range") != "" {
			sawRange = true
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	req := DownloadRequest{URL: server.URL + "/go.tar.gz", CacheDir: t.TempDir(), SDK: "go", Version: "1.21.1", Filename: "go.tar.gz"}
	partPath := sdkDownloadPartPath(req)
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(partPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := saveDownloadMeta(sdkDownloadMetaPath(req), downloadMeta{
		Schema:   1,
		URL:      req.URL,
		Filename: req.Filename,
		Size:     999,
		ETag:     `"v1"`,
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.False(t, sawRange)
	assert.False(t, result.Resumed)
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	assert.Eq(t, string(body), string(data))
}

func TestDownloadArchiveRestartsWhenRangeReturnsOK(t *testing.T) {
	body := []byte("full archive")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		if !strings.HasPrefix(r.Header.Get("Range"), "bytes=") {
			t.Fatalf("expected initial range request")
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	req := DownloadRequest{URL: server.URL + "/go.tar.gz", CacheDir: t.TempDir(), SDK: "go", Version: "1.21.1", Filename: "go.tar.gz"}
	partPath := sdkDownloadPartPath(req)
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(partPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := saveDownloadMeta(sdkDownloadMetaPath(req), downloadMeta{
		Schema:   1,
		URL:      req.URL,
		Filename: req.Filename,
		Size:     int64(len(body)),
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.False(t, result.Resumed)
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	assert.Eq(t, string(body), string(data))
}
