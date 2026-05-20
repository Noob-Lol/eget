package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

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
