package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
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
