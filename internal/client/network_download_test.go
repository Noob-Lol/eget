package client

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
