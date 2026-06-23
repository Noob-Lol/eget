package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inherelab/eget/internal/cachemirror"
	"github.com/inherelab/eget/internal/client"
)

const sdkTestChunkSize = 4 * 1024 * 1024

type sdkTestClientMeta struct {
	Schema    int                  `json:"schema"`
	URL       string               `json:"url"`
	Size      int64                `json:"size"`
	ETag      string               `json:"etag,omitempty"`
	ChunkSize int64                `json:"chunk_size,omitempty"`
	Chunks    []sdkTestClientChunk `json:"chunks,omitempty"`
}

type sdkTestClientChunk struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  bool  `json:"done"`
}

func TestNewDownloadHTTPClientPropagatesProxyExclude(t *testing.T) {
	httpClient, err := newDownloadHTTPClient(client.Options{
		ProxyURL:     "http://127.0.0.1:7890",
		ProxyExclude: []string{"github.com"},
	})
	assert.NoErr(t, err)

	transport, ok := httpClient.Transport.(*http.Transport)
	assert.True(t, ok)
	excludedReq := httptest.NewRequest(http.MethodGet, "https://api.github.com/repos/owner/repo", nil)
	got, err := transport.Proxy(excludedReq)
	assert.NoErr(t, err)
	assert.Eq(t, (*url.URL)(nil), got)

	allowedReq := httptest.NewRequest(http.MethodGet, "https://example.com/archive.tar.gz", nil)
	got, err = transport.Proxy(allowedReq)
	assert.NoErr(t, err)
	assert.Eq(t, "http://127.0.0.1:7890", got.String())
}

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

func TestDownloadArchiveUsesCompleteCacheWhenOnlyMetaURLDiffers(t *testing.T) {
	var originHit bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHit = true
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	cacheDir := t.TempDir()
	req := DownloadRequest{
		URL:      origin.URL + "/go.tar.gz",
		CacheDir: cacheDir,
		SDK:      "go",
		Version:  "1.21.13",
		Filename: "go1.21.13.linux-amd64.tar.gz",
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
		URL:      "https://mirrors.aliyun.com/golang/go1.21.13.linux-amd64.tar.gz",
		Filename: req.Filename,
		Size:     int64(len("archive")),
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.False(t, originHit)
	assert.True(t, result.FromCache)
	assert.Eq(t, finalPath, result.Path)
	assert.Eq(t, int64(len("archive")), result.Size)
}

func TestDownloadArchiveUsesCacheMirrorBeforeOrigin(t *testing.T) {
	var originHit bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originHit = true
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	cacheDir := t.TempDir()
	req := DownloadRequest{
		URL:      origin.URL + "/go.zip",
		CacheDir: cacheDir,
		SDK:      "go",
		Version:  "1.22.0",
		Filename: "go.zip",
	}
	rel, err := cachemirror.RelPath(cacheDir, sdkDownloadFinalPath(req))
	assert.NoErr(t, err)
	expectedPath := "/download/" + cachemirror.KeyForRelPath(rel)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Eq(t, expectedPath, r.URL.Path)
		_, _ = w.Write([]byte("mirror"))
	}))
	defer mirror.Close()
	req.CacheMirror = cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: true}

	result, err := DownloadArchive(context.Background(), req)

	assert.NoErr(t, err)
	assert.False(t, originHit)
	assert.Eq(t, sdkDownloadFinalPath(req), result.Path)
	assert.True(t, result.FromCache)
	data, err := os.ReadFile(result.Path)
	assert.NoErr(t, err)
	assert.Eq(t, "mirror", string(data))
}

func TestDownloadArchiveCacheMirrorMissFallsBack(t *testing.T) {
	mirror := httptest.NewServer(http.NotFoundHandler())
	defer mirror.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	req := DownloadRequest{
		URL:         origin.URL + "/go.zip",
		CacheDir:    t.TempDir(),
		SDK:         "go",
		Version:     "1.22.0",
		Filename:    "go.zip",
		CacheMirror: cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: true},
	}

	result, err := DownloadArchive(context.Background(), req)

	assert.NoErr(t, err)
	assert.Eq(t, int64(len("origin")), result.Size)
}

func TestDownloadArchiveCacheMirrorMissErrorsWhenFallbackDisabled(t *testing.T) {
	mirror := httptest.NewServer(http.NotFoundHandler())
	defer mirror.Close()

	req := DownloadRequest{
		URL:         "https://example.com/go.zip",
		CacheDir:    t.TempDir(),
		SDK:         "go",
		Version:     "1.22.0",
		Filename:    "go.zip",
		CacheMirror: cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: false},
	}

	_, err := DownloadArchive(context.Background(), req)

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "cache mirror miss")
}

func TestDownloadArchiveCacheMirrorHitWritesReusableMeta(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("mirror"))
	}))
	defer mirror.Close()
	req := DownloadRequest{
		URL:         mirror.URL + "/go.zip",
		CacheDir:    t.TempDir(),
		SDK:         "go",
		Version:     "1.22.0",
		Filename:    "go.zip",
		CacheMirror: cachemirror.Options{Enable: true, URL: mirror.URL, Fallback: true},
	}

	_, err := DownloadArchive(context.Background(), req)
	assert.NoErr(t, err)
	ok, meta := completeCacheMatches(sdkDownloadFinalPath(req), sdkDownloadMetaPath(req), req)
	assert.True(t, ok)
	assert.Eq(t, req.URL, meta.URL)
}

func TestDownloadArchiveUsesParallelClientDownload(t *testing.T) {
	body := bytes.Repeat([]byte("s"), 12*1024*1024)
	var rangeRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"sdk-parallel-v1"`)
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = w.Write(body)
			return
		}
		rangeRequests.Add(1)
		start, end := parseSDKTestRange(t, rangeHeader)
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
	}))
	defer server.Close()

	req := DownloadRequest{
		URL:        server.URL + "/go.tar.gz",
		CacheDir:   t.TempDir(),
		SDK:        "go",
		Version:    "1.21.1",
		Filename:   "go.tar.gz",
		ClientOpts: client.Options{ChunkConcurrency: 3},
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.Eq(t, sdkDownloadFinalPath(req), result.Path)
	assert.Eq(t, int64(len(body)), result.Size)
	assert.Eq(t, `"sdk-parallel-v1"`, result.ETag)
	assert.True(t, rangeRequests.Load() >= 3)
	saved, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if !bytes.Equal(body, saved) {
		t.Fatalf("downloaded archive mismatch: got %d bytes", len(saved))
	}
}

func TestDownloadArchiveResumesMissingChunksWhenMetaMatches(t *testing.T) {
	body := bytes.Repeat([]byte("r"), 12*1024*1024)
	chunks := sdkTestChunks(len(body), 3)
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
			expected := fmt.Sprintf("bytes=%d-%d", chunks[2].Start, chunks[2].End)
			if rangeHeader != expected {
				t.Fatalf("unexpected range header %q", rangeHeader)
			}
			w.Header().Set("Content-Length", fmt.Sprint(chunks[2].End-chunks[2].Start+1))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", chunks[2].Start, chunks[2].End, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[chunks[2].Start : chunks[2].End+1])
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	req := DownloadRequest{URL: server.URL + "/go.tar.gz", CacheDir: t.TempDir(), SDK: "go", Version: "1.21.1", Filename: "go.tar.gz", ClientOpts: client.Options{ChunkConcurrency: 3}}
	finalPath := sdkDownloadFinalPath(req)
	partPath := finalPath + ".part"
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	part := make([]byte, len(body))
	copy(part[chunks[0].Start:chunks[0].End+1], body[chunks[0].Start:chunks[0].End+1])
	copy(part[chunks[1].Start:chunks[1].End+1], body[chunks[1].Start:chunks[1].End+1])
	if err := os.WriteFile(partPath, part, 0o644); err != nil {
		t.Fatalf("write part: %v", err)
	}
	chunks[0].Done = true
	chunks[1].Done = true
	if err := saveSDKTestClientMeta(sdkDownloadMetaPath(req), sdkTestClientMeta{
		Schema:    2,
		URL:       req.URL,
		Size:      int64(len(body)),
		ETag:      `"v1"`,
		ChunkSize: sdkTestChunkSize,
		Chunks:    chunks,
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.Eq(t, fmt.Sprintf("bytes=%d-%d", chunks[2].Start, chunks[2].End), rangeHeader)
	assert.True(t, result.Resumed)
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if !bytes.Equal(body, data) {
		t.Fatalf("downloaded archive mismatch: got %d bytes", len(data))
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Fatalf("expected part file to be renamed, stat err=%v", err)
	}
}

func parseSDKTestRange(t *testing.T, header string) (int, int) {
	t.Helper()
	header = strings.TrimPrefix(header, "bytes=")
	parts := strings.Split(header, "-")
	if len(parts) != 2 {
		t.Fatalf("invalid range header %q", header)
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatalf("invalid range start %q: %v", parts[0], err)
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("invalid range end %q: %v", parts[1], err)
	}
	return start, end
}

func sdkTestChunks(size, chunks int) []sdkTestClientChunk {
	step := size / chunks
	metas := make([]sdkTestClientChunk, 0, chunks)
	start := 0
	for i := 0; i < chunks; i++ {
		end := start + step - 1
		if i == chunks-1 {
			end = size - 1
		}
		metas = append(metas, sdkTestClientChunk{Start: int64(start), End: int64(end)})
		start = end + 1
	}
	return metas
}

func saveSDKTestClientMeta(path string, meta sdkTestClientMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func TestDownloadArchiveRestartsWhenETagChanges(t *testing.T) {
	body := bytes.Repeat([]byte("n"), 12*1024*1024)
	var rangeRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"v2"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			rangeRequests.Add(1)
			start, end := parseSDKTestRange(t, rangeHeader)
			w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[start : end+1])
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	req := DownloadRequest{URL: server.URL + "/go.tar.gz", CacheDir: t.TempDir(), SDK: "go", Version: "1.21.1", Filename: "go.tar.gz", ClientOpts: client.Options{ChunkConcurrency: 3}}
	finalPath := sdkDownloadFinalPath(req)
	partPath := finalPath + ".part"
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(partPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := saveSDKTestClientMeta(sdkDownloadMetaPath(req), sdkTestClientMeta{
		Schema:    2,
		URL:       req.URL,
		Size:      int64(len(body)),
		ETag:      `"v1"`,
		ChunkSize: sdkTestChunkSize,
		Chunks:    sdkTestChunks(len(body), 3),
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.True(t, rangeRequests.Load() >= 3)
	assert.False(t, result.Resumed)
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if !bytes.Equal(body, data) {
		t.Fatalf("downloaded archive mismatch: got %d bytes", len(data))
	}
}

func TestDownloadArchiveRestartsWhenRangeReturnsOK(t *testing.T) {
	body := bytes.Repeat([]byte("o"), 12*1024*1024)
	var sawRange atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		if strings.HasPrefix(r.Header.Get("Range"), "bytes=") {
			sawRange.Store(true)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	req := DownloadRequest{URL: server.URL + "/go.tar.gz", CacheDir: t.TempDir(), SDK: "go", Version: "1.21.1", Filename: "go.tar.gz", ClientOpts: client.Options{ChunkConcurrency: 3}}
	finalPath := sdkDownloadFinalPath(req)
	partPath := finalPath + ".part"
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(partPath, make([]byte, len(body)), 0o644); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := saveSDKTestClientMeta(sdkDownloadMetaPath(req), sdkTestClientMeta{
		Schema:    2,
		URL:       req.URL,
		Size:      int64(len(body)),
		ChunkSize: sdkTestChunkSize,
		Chunks:    sdkTestChunks(len(body), 3),
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	result, err := DownloadArchive(context.Background(), req)
	if err != nil {
		t.Fatalf("download archive: %v", err)
	}

	assert.True(t, sawRange.Load())
	assert.False(t, result.Resumed)
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if !bytes.Equal(body, data) {
		t.Fatalf("downloaded archive mismatch: got %d bytes", len(data))
	}
}
