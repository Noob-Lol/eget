package install

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestCacheFilePath(t *testing.T) {
	cacheDir := t.TempDir()
	got := CacheFilePath(cacheDir, "https://github.com/babarot/gomi/releases/download/v1.6.3/gomi_Linux_x86_64.tar.gz")
	wantDir := filepath.Join(cacheDir, "pkg-cache")
	if filepath.Dir(got) != wantDir {
		t.Fatalf("expected cache file under %q, got %q", wantDir, got)
	}
	name := filepath.Base(got)
	if !strings.HasPrefix(name, "gomi_Linux_x86_64-1.6.3-") {
		t.Fatalf("expected readable cache name, got %q", name)
	}
	if !strings.HasSuffix(name, ".tar.gz") {
		t.Fatalf("expected extension .tar.gz, got %q", name)
	}
	if len(strings.TrimSuffix(strings.TrimPrefix(name, "gomi_Linux_x86_64-1.6.3-"), ".tar.gz")) != 8 {
		t.Fatalf("expected 8-char short hash in %q", name)
	}
}

func TestCacheFilePathUsesMetadataForOpaqueURL(t *testing.T) {
	cacheDir := t.TempDir()
	got := CacheFilePathWithMeta(cacheDir, "https://example.com/download?id=123", CacheMeta{
		Name:    "gomi",
		Version: "v1.6.3",
	})

	name := filepath.Base(got)
	if !strings.HasPrefix(name, "gomi-1.6.3-") {
		t.Fatalf("expected metadata cache name, got %q", name)
	}
	if !strings.HasSuffix(name, ".bin") {
		t.Fatalf("expected .bin fallback extension, got %q", name)
	}
}

func TestDownloadBodyUsesCacheWhenAvailable(t *testing.T) {
	cacheDir := t.TempDir()
	url := "https://example.com/tool.tar.gz"
	cachePath := CacheFilePath(cacheDir, url)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("cached-data"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	calls := 0
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("network-data")),
		}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &InstallRunner{Stdout: &stdout, Stderr: &stderr}
	downloaded, err := runner.downloadBody(url, Options{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("download body: %v", err)
	}

	if string(downloaded.Body) != "cached-data" {
		t.Fatalf("expected cached data, got %q", string(downloaded.Body))
	}
	if calls != 0 {
		t.Fatalf("expected no network calls, got %d", calls)
	}
	if got := stdout.String(); !strings.Contains(got, "Using cached file") {
		t.Fatalf("expected cached-file notice, got %q", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("expected cached-file notice stderr to be empty, got %q", got)
	}
}

func TestDownloadBodyUsesCachedFileWithoutRemoteProbe(t *testing.T) {
	cacheDir := t.TempDir()
	url := "https://github.com/pbatard/rufus/releases/download/v4.14/rufus-4.14p.exe"
	cachePath := CacheFilePath(cacheDir, url)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("cached-data"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	localTime := time.Date(2026, 5, 24, 13, 19, 24, 0, time.UTC)
	assert.Nil(t, applyModTime(cachePath, localTime))

	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		t.Fatalf("cache hit should not probe remote metadata with %s %s", req.Method, req.URL.String())
		return nil, nil
	}

	origDownloadGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origDownloadGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		t.Fatal("cache hit should not re-download body")
		return nil, nil
	}

	var stdout bytes.Buffer
	runner := &InstallRunner{Stdout: &stdout, Stderr: io.Discard}
	downloaded, err := runner.downloadBody(url, Options{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("download body: %v", err)
	}

	assert.Eq(t, "cached-data", string(downloaded.Body))
	assert.Eq(t, localTime, downloaded.ModTime.UTC())
	assert.Eq(t, localTime, fileModTime(cachePath).UTC())
	if got := stdout.String(); !strings.Contains(got, "Using cached file") {
		t.Fatalf("expected cached-file notice, got %q", got)
	}
}

func TestDownloadBodyRedownloadsHTMLCachedArchive(t *testing.T) {
	cacheDir := t.TempDir()
	url := "https://downloads.sourceforge.net/project/victoria-ssd-hdd/Victoria537.zip"
	cachePath := CacheFilePath(cacheDir, url)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("<!doctype html><html></html>"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	calls := 0
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(strings.NewReader("zip-data")),
			ContentLength: 8,
		}, nil
	}

	var stdout bytes.Buffer
	runner := &InstallRunner{Stdout: &stdout, Stderr: io.Discard}
	downloaded, err := runner.downloadBody(url, Options{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("download body: %v", err)
	}

	assert.Eq(t, "zip-data", string(downloaded.Body))
	assert.Eq(t, 1, calls)
	saved, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	assert.Eq(t, "zip-data", string(saved))
	assert.False(t, strings.Contains(stdout.String(), "Using cached file"))
}

func TestDownloadBodyWritesCacheAfterDownload(t *testing.T) {
	cacheDir := t.TempDir()
	url := "https://example.com/tool.tar.gz"
	cachePath := CacheFilePath(cacheDir, url)

	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("network-data")),
		}, nil
	}

	runner := &InstallRunner{Stderr: io.Discard}
	downloaded, err := runner.downloadBody(url, Options{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("download body: %v", err)
	}
	if string(downloaded.Body) != "network-data" {
		t.Fatalf("expected network data, got %q", string(downloaded.Body))
	}

	saved, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if string(saved) != "network-data" {
		t.Fatalf("expected cached network data, got %q", string(saved))
	}
}

func TestDownloadBodyUsesCacheMetadata(t *testing.T) {
	cacheDir := t.TempDir()
	url := "https://example.com/download?id=123"
	cachePath := CacheFilePathWithMeta(cacheDir, url, CacheMeta{Name: "gomi", Version: "v1.6.3"})
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("cached-data"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	calls := 0
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("network-data")),
		}, nil
	}

	runner := &InstallRunner{Stderr: io.Discard}
	downloaded, err := runner.downloadBody(url, Options{CacheDir: cacheDir, CacheName: "gomi", CacheVersion: "v1.6.3"})
	if err != nil {
		t.Fatalf("download body: %v", err)
	}
	if string(downloaded.Body) != "cached-data" {
		t.Fatalf("expected cached data, got %q", string(downloaded.Body))
	}
	if calls != 0 {
		t.Fatalf("expected no network calls, got %d", calls)
	}
}

func TestDownloadBodyResumesLargeCachedDownload(t *testing.T) {
	body := bytes.Repeat([]byte("r"), 12*1024*1024)
	chunkSize := 4 * 1024 * 1024
	chunkStart := 2 * chunkSize
	chunkEnd := len(body) - 1

	var gotRange atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"install-resume-v1"`)
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		rangeHeader := r.Header.Get("Range")
		gotRange.Store(rangeHeader)
		if rangeHeader != "" {
			if rangeHeader != fmt.Sprintf("bytes=%d-%d", chunkStart, chunkEnd) {
				t.Fatalf("unexpected range %q", rangeHeader)
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(body)-chunkStart))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", chunkStart, chunkEnd, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[chunkStart:])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	downloadURL := server.URL + "/tool.zip"

	cacheDir := t.TempDir()
	cachePath := CacheFilePathWithMeta(cacheDir, downloadURL, CacheMeta{})
	assert.Nil(t, os.MkdirAll(filepath.Dir(cachePath), 0o755))
	part := make([]byte, len(body))
	copy(part[:chunkStart], body[:chunkStart])
	assert.Nil(t, os.WriteFile(cachePath+".part", part, 0o644))
	meta := fmt.Sprintf(`{
  "schema": 2,
  "url": %q,
  "size": %d,
  "etag": %q,
  "chunk_size": %d,
  "chunks": [
    {"start": 0, "end": %d, "done": true},
    {"start": %d, "end": %d, "done": true},
    {"start": %d, "end": %d, "done": false}
  ]
}
`, downloadURL, len(body), `"install-resume-v1"`, chunkSize, chunkSize-1, chunkSize, chunkStart-1, chunkStart, chunkEnd)
	assert.Nil(t, os.WriteFile(cachePath+".meta.json", []byte(meta), 0o644))

	runner := &InstallRunner{Stderr: io.Discard}
	got, err := runner.downloadBody(downloadURL, Options{CacheDir: cacheDir})

	assert.Nil(t, err)
	assert.Eq(t, fmt.Sprintf("bytes=%d-%d", chunkStart, chunkEnd), gotRange.Load())
	assert.Eq(t, body, got.Body)
	saved, readErr := os.ReadFile(cachePath)
	assert.Nil(t, readErr)
	assert.Eq(t, body, saved)
	_, statErr := os.Stat(cachePath + ".part")
	assert.True(t, os.IsNotExist(statErr))
}

func TestDownloadPrintsProxyNoticeForRemoteRequest(t *testing.T) {
	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	origGetWithOptions := downloadGetWithOptions
	defer func() {
		proxyNoticeWriter = origNoticeWriter
		downloadGetWithOptions = origGetWithOptions
	}()
	proxyNoticeWriter = &notice
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		if opts.ProxyURL != "http://127.0.0.1:7890" {
			t.Fatalf("expected proxy url to propagate, got %q", opts.ProxyURL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("network-data")),
		}, nil
	}

	err := Download("https://example.com/tool.tar.gz", io.Discard, func(size int64) io.Writer {
		return io.Discard
	}, Options{ProxyURL: "http://127.0.0.1:7890"})
	if err != nil {
		t.Fatalf("Download(): %v", err)
	}

	if got := notice.String(); !strings.Contains(got, "proxy_url for download request") {
		t.Fatalf("expected download proxy notice, got %q", got)
	}
}

type recordingProgress struct {
	bytes    int
	finished bool
}

func (p *recordingProgress) Write(data []byte) (int, error) {
	p.bytes += len(data)
	return len(data), nil
}

func (p *recordingProgress) Finish(...string) {
	p.finished = true
}

func TestDownloadWritesAndFinishesProgressWriter(t *testing.T) {
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 12,
			Body:          io.NopCloser(strings.NewReader("network-data")),
		}, nil
	}

	progress := &recordingProgress{}
	var out bytes.Buffer
	err := Download("https://example.com/tool.tar.gz", &out, func(size int64) io.Writer {
		if size != 12 {
			t.Fatalf("expected content length 12, got %d", size)
		}
		return progress
	}, Options{})
	if err != nil {
		t.Fatalf("Download(): %v", err)
	}
	if out.String() != "network-data" {
		t.Fatalf("expected downloaded body, got %q", out.String())
	}
	if progress.bytes != len("network-data") {
		t.Fatalf("expected progress bytes %d, got %d", len("network-data"), progress.bytes)
	}
	if !progress.finished {
		t.Fatal("expected progress writer to be finished")
	}
}

func TestNewDownloadProgressUsesCoarseRedrawFrequency(t *testing.T) {
	p := newDownloadProgress(io.Discard, 500*1024*1024)
	defer p.Finish()

	assert.True(t, p.RedrawFreq >= 256*1024)
}

func TestDownloadProgressLayoutAdaptsToTerminalWidth(t *testing.T) {
	barWidth, format := downloadProgressLayout(120)
	assert.Eq(t, 40, barWidth)
	assert.Contains(t, format, "{@elapsed}/{@remaining}")
	assert.NotContains(t, format, "{@estimated}")

	barWidth, format = downloadProgressLayout(100)
	assert.Eq(t, 32, barWidth)
	assert.Contains(t, format, "{@elapsed}/{@remaining}")
	assert.NotContains(t, format, "{@estimated}")

	barWidth, format = downloadProgressLayout(80)
	assert.Eq(t, 24, barWidth)
	assert.Contains(t, format, "{@curSize}/{@maxSize}")
	assert.NotContains(t, format, "{@elapsed}")
	assert.NotContains(t, format, "{@remaining}")

	barWidth, _ = downloadProgressLayout(60)
	assert.Eq(t, 10, barWidth)
}

func TestDownloadSkipsProxyNoticeForLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "tool.tar.gz")
	if err := os.WriteFile(localFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	defer func() { proxyNoticeWriter = origNoticeWriter }()
	proxyNoticeWriter = &notice

	err := Download(localFile, io.Discard, func(size int64) io.Writer {
		return io.Discard
	}, Options{ProxyURL: "http://127.0.0.1:7890"})
	if err != nil {
		t.Fatalf("Download(local): %v", err)
	}

	if got := notice.String(); got != "" {
		t.Fatalf("expected no proxy notice for local file, got %q", got)
	}
}

func TestNewHTTPGetterUsesProxyURL(t *testing.T) {
	proxyFunc, err := proxyFuncFor("http://127.0.0.1:7890")
	if err != nil {
		t.Fatalf("proxyFuncFor: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/tool.tar.gz", nil)
	if err == nil {
		proxyURL, err := proxyFunc(req)
		if err != nil {
			t.Fatalf("proxy func: %v", err)
		}
		if proxyURL == nil {
			t.Fatal("expected proxy url to be returned")
		}
		if proxyURL.String() != "http://127.0.0.1:7890" {
			t.Fatalf("expected proxy url http://127.0.0.1:7890, got %q", proxyURL.String())
		}
		return
	}
	t.Fatalf("new request: %v", err)
}

func TestProxyFuncForRejectsInvalidProxyURL(t *testing.T) {
	_, err := proxyFuncFor("://bad-proxy")
	if err == nil {
		t.Fatal("expected invalid proxy url error")
	}
	if !strings.Contains(err.Error(), "invalid proxy_url") {
		t.Fatalf("expected invalid proxy_url error, got %v", err)
	}
}

func TestProxyFuncForFallsBackToEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7891")
	t.Setenv("https_proxy", "http://127.0.0.1:7891")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	t.Setenv("REQUEST_METHOD", "")
	proxyFunc, err := proxyFuncFor("")
	if err != nil {
		t.Fatalf("proxyFuncFor env fallback: %v", err)
	}
	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}
	proxyURL, err := proxyFunc(req)
	if err != nil {
		t.Fatalf("proxy func env fallback: %v", err)
	}
	if proxyURL == nil {
		t.Fatal("expected environment proxy url to be returned")
	}
	if proxyURL.String() != "http://127.0.0.1:7891" {
		t.Fatalf("expected env proxy url http://127.0.0.1:7891, got %q", proxyURL.String())
	}
}

func TestGetWithOptionsPrintsProxyNoticeForGitHubAPI(t *testing.T) {
	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	origHTTPDo := httpDo
	defer func() {
		proxyNoticeWriter = origNoticeWriter
		httpDo = origHTTPDo
	}()
	proxyNoticeWriter = &notice
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}

	resp, err := GetWithOptions("https://api.github.com/repos/gookit/gitw/releases/latest", Options{ProxyURL: "http://127.0.0.1:7890"})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	_ = resp.Body.Close()

	if got := notice.String(); !strings.Contains(got, "proxy_url for GitHub API request") {
		t.Fatalf("expected GitHub API proxy notice, got %q", got)
	}
}

func TestGetWithOptionsSkipsProxyNoticeWithoutProxyURL(t *testing.T) {
	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	origHTTPDo := httpDo
	defer func() {
		proxyNoticeWriter = origNoticeWriter
		httpDo = origHTTPDo
	}()
	proxyNoticeWriter = &notice
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}

	resp, err := GetWithOptions("https://api.github.com/repos/gookit/gitw/releases/latest", Options{})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	_ = resp.Body.Close()

	if got := notice.String(); got != "" {
		t.Fatalf("expected no proxy notice without proxy_url, got %q", got)
	}
}

func TestGetWithOptionsPrintsVerboseRequestAndResponse(t *testing.T) {
	var verbose bytes.Buffer
	origVerboseEnabled := verboseEnabled
	origVerboseWriter := verboseWriter
	origHTTPDo := httpDo
	defer func() {
		verboseEnabled = origVerboseEnabled
		verboseWriter = origVerboseWriter
		httpDo = origHTTPDo
	}()
	SetVerbose(true, &verbose)
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}

	resp, err := GetWithOptions("https://api.github.com/repos/gookit/gitw/releases/latest", Options{})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	_ = resp.Body.Close()

	got := verbose.String()
	if !strings.Contains(got, "request: GET https://api.github.com/repos/gookit/gitw/releases/latest") {
		t.Fatalf("expected verbose request log, got %q", got)
	}
	if !strings.Contains(got, "response: https://api.github.com/repos/gookit/gitw/releases/latest 200 OK") {
		t.Fatalf("expected verbose response log, got %q", got)
	}
}

func TestGetWithOptionsUsesAPICacheWhenAvailable(t *testing.T) {
	cacheDir := t.TempDir()
	apiURL := "https://api.github.com/repos/gookit/gitw/releases/latest"
	cachePath := APICacheFilePath(cacheDir, apiURL)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte(`{"tag_name":"v0.3.6"}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	calls := 0
	origHTTPDo := httpDo
	origNoticeWriter := apiCacheNoticeWriter
	defer func() { httpDo = origHTTPDo }()
	defer func() { apiCacheNoticeWriter = origNoticeWriter }()
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"network"}`)),
		}, nil
	}
	var notice bytes.Buffer
	apiCacheNoticeWriter = &notice

	resp, err := GetWithOptions(apiURL, Options{
		APICacheEnabled: true,
		APICacheDir:     cacheDir,
		APICacheTime:    300,
	})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != `{"tag_name":"v0.3.6"}` {
		t.Fatalf("expected cached response body, got %q", string(body))
	}
	if calls != 0 {
		t.Fatalf("expected no network calls, got %d", calls)
	}
	if got := notice.String(); !strings.Contains(got, "api_cache file") {
		t.Fatalf("expected api cache notice, got %q", got)
	}
}

func TestGetWithOptionsWritesAPICacheAfterNetworkRequest(t *testing.T) {
	cacheDir := t.TempDir()
	apiURL := "https://api.github.com/repos/gookit/gitw/releases/latest"

	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v0.3.6"}`)),
		}, nil
	}

	resp, err := GetWithOptions(apiURL, Options{
		APICacheEnabled: true,
		APICacheDir:     cacheDir,
		APICacheTime:    300,
	})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	cachePath := APICacheFilePath(cacheDir, apiURL)
	saved, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if string(saved) != `{"tag_name":"v0.3.6"}` {
		t.Fatalf("expected cached response body, got %q", string(saved))
	}
}

func TestGetWithOptionsUsesGhproxyForDownloads(t *testing.T) {
	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()

	var requested string
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		requested = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	}

	resp, err := GetWithOptions("https://github.com/gookit/gitw/releases/download/v0.3.6/chlog-windows-amd64.exe", Options{
		GhproxyEnabled: true,
		GhproxyHostURL: "https://gh.felicity.ac.cn",
	})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	_ = resp.Body.Close()

	want := "https://gh.felicity.ac.cn/https://github.com/gookit/gitw/releases/download/v0.3.6/chlog-windows-amd64.exe"
	if requested != want {
		t.Fatalf("expected ghproxy rewritten url %q, got %q", want, requested)
	}
}

func TestGetWithOptionsUsesGhproxyForGitHubAPIWhenSupported(t *testing.T) {
	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()

	var requested string
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		requested = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}

	resp, err := GetWithOptions("https://api.github.com/repos/gookit/gitw/releases/latest", Options{
		GhproxyEnabled:    true,
		GhproxyHostURL:    "https://gh.felicity.ac.cn",
		GhproxySupportAPI: true,
	})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	_ = resp.Body.Close()

	want := "https://gh.felicity.ac.cn/https://api.github.com/repos/gookit/gitw/releases/latest"
	if requested != want {
		t.Fatalf("expected ghproxy rewritten api url %q, got %q", want, requested)
	}
}

func TestGetWithOptionsFallsBackToNextGhproxyHost(t *testing.T) {
	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()

	var requested []string
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		requested = append(requested, req.URL.String())
		if strings.Contains(req.URL.Host, "gh.felicity.ac.cn") {
			return nil, io.EOF
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	}

	resp, err := GetWithOptions("https://github.com/gookit/gitw/releases/download/v0.3.6/chlog-windows-amd64.exe", Options{
		GhproxyEnabled:   true,
		GhproxyHostURL:   "https://gh.felicity.ac.cn",
		GhproxyFallbacks: []string{"https://gh.llkk.cc"},
	})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	_ = resp.Body.Close()

	if len(requested) != 2 {
		t.Fatalf("expected 2 ghproxy attempts, got %#v", requested)
	}
	if !strings.Contains(requested[1], "gh.llkk.cc") {
		t.Fatalf("expected fallback ghproxy host, got %#v", requested)
	}
}
