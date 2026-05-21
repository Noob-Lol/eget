package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	"github.com/gookit/goutil/x/ccolor"
)

func TestDownloadUsesRangeChunksForLargeFiles(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 10*1024*1024)
	var rangeRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		if header := r.Header.Get("Range"); header != "" {
			rangeRequests.Add(1)
			start, end := parseTestRange(t, header)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[start : end+1])
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Download(server.URL, &out, nil, Options{ChunkConcurrency: 4})
	assert.Nil(t, err)
	assert.Eq(t, body, out.Bytes())
	assert.True(t, rangeRequests.Load() > 1)
}

func TestDownloadFileKeepsPartForLargeRangeDownloadFailure(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 512*1024)
	remoteSize := int64(len(body) + 1024)
	origMinSize := resumableDownloadMinSize
	resumableDownloadMinSize = 256 * 1024
	defer func() { resumableDownloadMinSize = origMinSize }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.FormatInt(remoteSize, 10))
			w.Header().Set("ETag", `"large-v1"`)
			return
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.FormatInt(remoteSize, 10))
		w.Header().Set("ETag", `"large-v1"`)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tool.zip")
	_, err := DownloadFile(server.URL, target, nil, Options{ChunkConcurrency: 1})

	assert.NotNil(t, err)
	partInfo, statErr := os.Stat(target + ".part")
	assert.Nil(t, statErr)
	assert.True(t, partInfo.Size() > 0)
	_, statErr = os.Stat(target + ".meta.json")
	assert.Nil(t, statErr)
	_, statErr = os.Stat(target)
	assert.True(t, os.IsNotExist(statErr))
}

func TestDownloadFileUsesParallelRangeChunksForLargeFiles(t *testing.T) {
	body := bytes.Repeat([]byte("p"), 12*1024*1024)
	origMinSize := resumableDownloadMinSize
	resumableDownloadMinSize = 256 * 1024
	defer func() { resumableDownloadMinSize = origMinSize }()

	var rangeRequests atomic.Int64
	seenRanges := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"parallel-v1"`)
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
		seenRanges <- rangeHeader
		start, end := parseTestRange(t, rangeHeader)
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tool.zip")
	result, err := DownloadFile(server.URL+"/tool.zip", target, nil, Options{ChunkConcurrency: 3})

	assert.Nil(t, err)
	assert.True(t, result.Parallel)
	assert.False(t, result.Resumed)
	assert.True(t, rangeRequests.Load() >= 3)
	saved, readErr := os.ReadFile(target)
	assert.Nil(t, readErr)
	assert.Eq(t, body, saved)
	_, statErr := os.Stat(target + ".part")
	assert.True(t, os.IsNotExist(statErr))

	close(seenRanges)
	unique := map[string]struct{}{}
	for rangeHeader := range seenRanges {
		unique[rangeHeader] = struct{}{}
	}
	assert.True(t, len(unique) >= 3)
}

func TestDownloadFileResumesOnlyMissingChunks(t *testing.T) {
	body := bytes.Repeat([]byte("m"), 12*1024*1024)
	origMinSize := resumableDownloadMinSize
	resumableDownloadMinSize = 256 * 1024
	defer func() { resumableDownloadMinSize = origMinSize }()

	chunks := planDownloadChunks(int64(len(body)), 3)
	var requested atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"missing-v1"`)
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == fmt.Sprintf("bytes=%d-%d", chunks[0].Start, chunks[0].End) ||
			rangeHeader == fmt.Sprintf("bytes=%d-%d", chunks[1].Start, chunks[1].End) {
			t.Fatalf("completed chunk requested again: %s", rangeHeader)
		}
		assert.Eq(t, fmt.Sprintf("bytes=%d-%d", chunks[2].Start, chunks[2].End), rangeHeader)
		requested.Add(1)
		start, end := parseTestRange(t, rangeHeader)
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tool.zip")
	part := make([]byte, len(body))
	copy(part[chunks[0].Start:chunks[0].End+1], body[chunks[0].Start:chunks[0].End+1])
	copy(part[chunks[1].Start:chunks[1].End+1], body[chunks[1].Start:chunks[1].End+1])
	assert.Nil(t, os.WriteFile(target+".part", part, 0o644))
	chunks[0].Done = true
	chunks[1].Done = true
	assert.Nil(t, saveDownloadFileMeta(target+".meta.json", downloadFileMeta{
		Schema:    2,
		URL:       server.URL + "/tool.zip",
		Size:      int64(len(body)),
		ETag:      `"missing-v1"`,
		ChunkSize: minChunkSize,
		Chunks:    chunks,
	}))

	result, err := DownloadFile(server.URL+"/tool.zip", target, nil, Options{ChunkConcurrency: 3})

	assert.Nil(t, err)
	assert.True(t, result.Resumed)
	assert.True(t, result.Parallel)
	assert.Eq(t, int64(1), requested.Load())
	saved, readErr := os.ReadFile(target)
	assert.Nil(t, readErr)
	assert.Eq(t, body, saved)
}

func TestDownloadFileRestartsWhenRangeChunkReturnsOK(t *testing.T) {
	body := bytes.Repeat([]byte("o"), 12*1024*1024)
	origMinSize := resumableDownloadMinSize
	resumableDownloadMinSize = 256 * 1024
	defer func() { resumableDownloadMinSize = origMinSize }()

	var sawRange atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"ok-v1"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		if r.Header.Get("Range") != "" {
			sawRange.Store(true)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tool.zip")
	chunks := planDownloadChunks(int64(len(body)), 3)
	assert.Nil(t, os.WriteFile(target+".part", make([]byte, len(body)), 0o644))
	assert.Nil(t, saveDownloadFileMeta(target+".meta.json", downloadFileMeta{
		Schema:    2,
		URL:       server.URL + "/tool.zip",
		Size:      int64(len(body)),
		ETag:      `"ok-v1"`,
		ChunkSize: minChunkSize,
		Chunks:    chunks,
	}))

	result, err := DownloadFile(server.URL+"/tool.zip", target, nil, Options{ChunkConcurrency: 3})

	assert.Nil(t, err)
	assert.True(t, sawRange.Load())
	assert.False(t, result.Resumed)
	saved, readErr := os.ReadFile(target)
	assert.Nil(t, readErr)
	assert.Eq(t, body, saved)
	_, statErr := os.Stat(target + ".part")
	assert.True(t, os.IsNotExist(statErr))
}

func TestDownloadFileKeepsCompletedChunksWhenOneChunkFails(t *testing.T) {
	body := bytes.Repeat([]byte("f"), 12*1024*1024)
	origMinSize := resumableDownloadMinSize
	resumableDownloadMinSize = 256 * 1024
	defer func() { resumableDownloadMinSize = origMinSize }()

	chunks := planDownloadChunks(int64(len(body)), 3)
	failedRange := fmt.Sprintf("bytes=%d-%d", chunks[2].Start, chunks[2].End)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"fail-v1"`)
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		rangeHeader := r.Header.Get("Range")
		start, end := parseTestRange(t, rangeHeader)
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		if rangeHeader == failedRange {
			_, _ = w.Write(body[start : start+1024])
			return
		}
		_, _ = w.Write(body[start : end+1])
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tool.zip")
	_, err := DownloadFile(server.URL+"/tool.zip", target, nil, Options{ChunkConcurrency: 3})

	assert.NotNil(t, err)
	_, statErr := os.Stat(target + ".part")
	assert.Nil(t, statErr)
	meta, ok := loadDownloadFileMeta(target + ".meta.json")
	assert.True(t, ok)
	done := 0
	notDone := 0
	for _, chunk := range meta.Chunks {
		if chunk.Done {
			done++
		} else {
			notDone++
		}
	}
	assert.True(t, done > 0)
	assert.True(t, notDone > 0)
}

func TestDownloadFileResumesLargeRangeDownload(t *testing.T) {
	body := bytes.Repeat([]byte("r"), 768*1024)
	partSize := len(body) / 2
	origMinSize := resumableDownloadMinSize
	resumableDownloadMinSize = 256 * 1024
	defer func() { resumableDownloadMinSize = origMinSize }()

	var gotRange atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"resume-v1"`)
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		rangeHeader := r.Header.Get("Range")
		gotRange.Store(rangeHeader)
		if rangeHeader != "" {
			assert.Eq(t, fmt.Sprintf("bytes=%d-", partSize), rangeHeader)
			w.Header().Set("Content-Length", strconv.Itoa(len(body)-partSize))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", partSize, len(body)-1, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[partSize:])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tool.zip")
	assert.Nil(t, os.WriteFile(target+".part", body[:partSize], 0o644))
	assert.Nil(t, saveDownloadFileMeta(target+".meta.json", downloadFileMeta{
		Schema: 1,
		URL:    server.URL,
		Size:   int64(len(body)),
		ETag:   `"resume-v1"`,
	}))

	_, err := DownloadFile(server.URL, target, nil, Options{ChunkConcurrency: 1})

	assert.Nil(t, err)
	assert.Eq(t, fmt.Sprintf("bytes=%d-", partSize), gotRange.Load())
	saved, readErr := os.ReadFile(target)
	assert.Nil(t, readErr)
	assert.Eq(t, body, saved)
	_, statErr := os.Stat(target + ".part")
	assert.True(t, os.IsNotExist(statErr))
}

func TestDownloadFileRemovesPartWhenLargeDownloadCannotResume(t *testing.T) {
	body := bytes.Repeat([]byte("n"), 512*1024)
	remoteSize := int64(len(body) + 1024)
	origMinSize := resumableDownloadMinSize
	resumableDownloadMinSize = 256 * 1024
	defer func() { resumableDownloadMinSize = origMinSize }()

	var gotRange atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(remoteSize, 10))
		gotRange.Store(r.Header.Get("Range"))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tool.zip")
	_, err := DownloadFile(server.URL, target, nil, Options{ChunkConcurrency: 1})

	assert.NotNil(t, err)
	assert.Eq(t, "", gotRange.Load())
	_, statErr := os.Stat(target + ".part")
	assert.True(t, os.IsNotExist(statErr))
	_, statErr = os.Stat(target + ".meta.json")
	assert.True(t, os.IsNotExist(statErr))
}

func TestProxyNoticePrintsOncePerKindAndProxyURL(t *testing.T) {
	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	origHTTPDo := httpDo
	origDownloadGet := downloadGetWithOptions
	defer func() {
		proxyNoticeWriter = origNoticeWriter
		httpDo = origHTTPDo
		downloadGetWithOptions = origDownloadGet
		resetProxyNoticeStateForTest()
	}()
	proxyNoticeWriter = &notice
	resetProxyNoticeStateForTest()
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`download`)),
		}, nil
	}

	opts := Options{ProxyURL: "http://127.0.0.1:7890", ChunkConcurrency: 1}
	for i := 0; i < 2; i++ {
		resp, err := GetWithOptions("https://api.github.com/repos/gookit/gitw/releases/latest", opts)
		assert.NoErr(t, err)
		_ = resp.Body.Close()
		assert.NoErr(t, Download("https://example.com/tool.tar.gz", io.Discard, func(int64) io.Writer {
			return io.Discard
		}, opts))
	}

	got := ccolor.ClearCode(notice.String())
	assert.Eq(t, 1, strings.Count(got, "proxy_url for GitHub API request"))
	assert.Eq(t, 1, strings.Count(got, "proxy_url for download request"))
}

func TestDownloadProgressFlushIntervalIsThrottled(t *testing.T) {
	assert.True(t, downloadProgressFlushInterval >= 500*time.Millisecond)
}

func TestDownloadRangeChunksUpdatesProgressWhileReading(t *testing.T) {
	body := bytes.Repeat([]byte("p"), 9*1024*1024)
	var rangeRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		if header := r.Header.Get("Range"); header != "" {
			rangeRequests.Add(1)
			start, end := parseTestRange(t, header)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			flusher, _ := w.(http.Flusher)
			for offset := start; offset <= end; {
				next := offset + 64*1024
				if next > end+1 {
					next = end + 1
				}
				_, _ = w.Write(body[offset:next])
				if flusher != nil {
					flusher.Flush()
				}
				offset = next
			}
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	progress := &recordingDownloadProgress{}
	var out bytes.Buffer
	err := Download(server.URL, &out, func(size int64) io.Writer {
		assert.Eq(t, int64(len(body)), size)
		return progress
	}, Options{ChunkConcurrency: 2})

	assert.Nil(t, err)
	assert.Eq(t, body, out.Bytes())
	assert.Eq(t, int64(2), rangeRequests.Load())
	assert.True(t, progress.writes.Load() > rangeRequests.Load())
	assert.Eq(t, int64(len(body)), progress.bytes.Load())
}

func TestDownloadSkipsRangeChunksForSmallFiles(t *testing.T) {
	body := bytes.Repeat([]byte("b"), 1024*1024)
	var rangeRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		if r.Header.Get("Range") != "" {
			rangeRequests.Add(1)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Download(server.URL, &out, nil, Options{ChunkConcurrency: 8})
	assert.Nil(t, err)
	assert.Eq(t, body, out.Bytes())
	assert.Eq(t, int64(0), rangeRequests.Load())
}

func TestDownloadFallsBackWhenServerDoesNotSupportRange(t *testing.T) {
	body := bytes.Repeat([]byte("c"), 10*1024*1024)
	var rangeRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		if r.Header.Get("Range") != "" {
			rangeRequests.Add(1)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Download(server.URL, &out, nil, Options{ChunkConcurrency: 4})
	assert.Nil(t, err)
	assert.Eq(t, body, out.Bytes())
	assert.Eq(t, int64(0), rangeRequests.Load())
}

func TestDownloadRangeChunksRejectsSizeAboveIntMax(t *testing.T) {
	if strconv.IntSize > 32 {
		t.Skip("requires a 32-bit int platform")
	}
	size := int64(1) << 31
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected an error instead of panic: %v", recovered)
		}
	}()

	err := downloadRangeChunks("https://example.com/tool", &bytes.Buffer{}, nil, size, 2, Options{})

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "too large")
}

type recordingDownloadProgress struct {
	writes atomic.Int64
	bytes  atomic.Int64
}

func (p *recordingDownloadProgress) Write(data []byte) (int, error) {
	p.writes.Add(1)
	p.bytes.Add(int64(len(data)))
	return len(data), nil
}

func parseTestRange(t *testing.T, header string) (int, int) {
	t.Helper()
	value := strings.TrimPrefix(header, "bytes=")
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		t.Fatalf("invalid range header %q", header)
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatalf("parse range start: %v", err)
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("parse range end: %v", err)
	}
	return start, end
}
