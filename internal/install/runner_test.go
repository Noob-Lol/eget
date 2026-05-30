package install

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	"github.com/gookit/goutil/x/ccolor"
	sourcegithub "github.com/inherelab/eget/internal/source/github"
	sourcesf "github.com/inherelab/eget/internal/source/sourceforge"
	"github.com/inherelab/eget/internal/source/urltemplate"
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

func TestRunDownloadOnlyPreservesLastModifiedTimestamp(t *testing.T) {
	modTime := time.Date(2026, 4, 30, 11, 32, 59, 0, time.UTC)
	body := []byte("asset-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", modTime.Format(http.TimeFormat))
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	outputDir := t.TempDir()
	runner := NewRunner(NewDefaultService(nil, nil))
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard

	result, err := runner.Run(server.URL+"/rufus-4.14p.exe", Options{
		DownloadOnly: true,
		Output:       outputDir,
		CacheDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("download asset: %v", err)
	}

	assert.Eq(t, 1, len(result.ExtractedFiles))
	info, err := os.Stat(result.ExtractedFiles[0])
	if err != nil {
		t.Fatalf("stat downloaded file: %v", err)
	}
	assert.Eq(t, modTime, info.ModTime().UTC())
}

func TestRunDownloadOnlyPreservesLastModifiedTimestampWithoutCache(t *testing.T) {
	modTime := time.Date(2026, 4, 30, 11, 32, 59, 0, time.UTC)
	body := []byte("asset-data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", modTime.Format(http.TimeFormat))
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	outputDir := t.TempDir()
	runner := NewRunner(NewDefaultService(nil, nil))
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard

	result, err := runner.Run(server.URL+"/rufus-4.14p.exe", Options{
		DownloadOnly: true,
		Output:       outputDir,
	})
	if err != nil {
		t.Fatalf("download asset: %v", err)
	}

	assert.Eq(t, 1, len(result.ExtractedFiles))
	info, err := os.Stat(result.ExtractedFiles[0])
	if err != nil {
		t.Fatalf("stat downloaded file: %v", err)
	}
	assert.Eq(t, modTime, info.ModTime().UTC())
}

func TestRunExtractFilePreservesArchiveDirectoryTimestamp(t *testing.T) {
	dirTime := time.Date(2024, 4, 5, 6, 7, 8, 0, time.UTC)
	remoteTime := time.Date(2026, 4, 30, 11, 32, 59, 0, time.UTC)
	var body bytes.Buffer
	zw := zip.NewWriter(&body)
	dirHeader := &zip.FileHeader{Name: "CdmResource/", Method: zip.Store, Modified: dirTime}
	dirHeader.SetMode(0o755 | os.ModeDir)
	if _, err := zw.CreateHeader(dirHeader); err != nil {
		t.Fatalf("create zip dir: %v", err)
	}
	fileHeader := &zip.FileHeader{Name: "CdmResource/resource.txt", Method: zip.Store, Modified: dirTime}
	fileHeader.SetMode(0o644)
	w, err := zw.CreateHeader(fileHeader)
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}
	if _, err := w.Write([]byte("resource")); err != nil {
		t.Fatalf("write zip file: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteTime.Format(http.TimeFormat))
		w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
		_, _ = w.Write(body.Bytes())
	}))
	defer server.Close()

	outputDir := t.TempDir()
	runner := NewRunner(NewDefaultService(nil, nil))
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard

	result, err := runner.Run(server.URL+"/app.zip", Options{
		DownloadOnly: true,
		ExtractFile:  "CdmResource",
		Output:       outputDir,
		CacheDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("extract directory: %v", err)
	}

	assert.Eq(t, 1, len(result.ExtractedFiles))
	info, err := os.Stat(filepath.Join(outputDir, "CdmResource"))
	if err != nil {
		t.Fatalf("stat extracted directory: %v", err)
	}
	assert.Eq(t, dirTime, info.ModTime().UTC())
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

func TestEffectiveOutputUsesGuiTargetForPortableGUI(t *testing.T) {
	opts := Options{Output: "C:/Tools", GuiTarget: "C:/Program/AITools", IsGUI: true, InstallMode: InstallModePortable}
	got := effectiveOutput(opts)
	if got != "C:/Program/AITools" {
		t.Fatalf("expected gui target, got %q", got)
	}
}

func TestEffectiveOutputKeepsExplicitOutputForPortableGUI(t *testing.T) {
	opts := Options{Output: "D:/Custom/PicoClaw", GuiTarget: "C:/Program/AITools", IsGUI: true, InstallMode: InstallModePortable, OutputExplicit: true}
	got := effectiveOutput(opts)
	if got != "D:/Custom/PicoClaw" {
		t.Fatalf("expected explicit output, got %q", got)
	}
}

type fakeInstallerLauncher struct {
	path string
	kind InstallerKind
	err  error
}

func (f *fakeInstallerLauncher) LaunchInstaller(path string, kind InstallerKind) error {
	f.path = path
	f.kind = kind
	return f.err
}

func TestLaunchGUIInstallerReturnsInstallerResult(t *testing.T) {
	launcher := &fakeInstallerLauncher{}
	runner := &InstallRunner{InstallerLauncher: launcher}
	file := ExtractedFile{Name: "PicoClaw-Setup.exe", ArchiveName: "PicoClaw-Setup.exe"}
	path := filepath.Join(t.TempDir(), "PicoClaw-Setup.exe")
	if err := os.WriteFile(path, []byte("installer"), 0o755); err != nil {
		t.Fatalf("write installer: %v", err)
	}
	result, err := runner.launchGUIInstaller(path, file, Options{IsGUI: true})
	if err != nil {
		t.Fatalf("launch gui installer: %v", err)
	}
	if result.InstallMode != InstallModeInstaller || !result.IsGUI || result.InstallerFile != path {
		t.Fatalf("expected installer gui result, got %#v", result)
	}
	if launcher.path != path || launcher.kind != InstallerKindEXE {
		t.Fatalf("unexpected launcher call path=%q kind=%q", launcher.path, launcher.kind)
	}
}

func TestRunPromptsBeforeLaunchingDetectedInstallerWithoutGUIFlag(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "PicoClaw-Setup.exe")
	if err := os.WriteFile(path, []byte("installer"), 0o755); err != nil {
		t.Fatalf("write installer: %v", err)
	}

	launcher := &fakeInstallerLauncher{}
	runner := NewRunner(NewDefaultService(nil, nil))
	runner.InstallerLauncher = launcher
	runner.Stderr = io.Discard
	var prompted string
	runner.ConfirmLaunchInstaller = func(file string) (bool, error) {
		prompted = file
		return true, nil
	}

	result, err := runner.Run(path, Options{})
	if err != nil {
		t.Fatalf("run installer: %v", err)
	}
	if prompted != "PicoClaw-Setup.exe" {
		t.Fatalf("expected setup file prompt, got %q", prompted)
	}
	if launcher.path != path || launcher.kind != InstallerKindEXE {
		t.Fatalf("unexpected launcher call path=%q kind=%q", launcher.path, launcher.kind)
	}
	if !result.IsGUI || result.InstallMode != InstallModeInstaller {
		t.Fatalf("expected confirmed installer result, got %#v", result)
	}
}

func TestRunLaunchesConfiguredInstallerModeForPlainExe(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "GoNavi-0.7.7-Windows-Amd64.exe")
	if err := os.WriteFile(path, []byte("installer"), 0o755); err != nil {
		t.Fatalf("write installer: %v", err)
	}

	launcher := &fakeInstallerLauncher{}
	runner := NewRunner(NewDefaultService(nil, nil))
	runner.InstallerLauncher = launcher
	runner.Stderr = io.Discard
	runner.ConfirmLaunchInstaller = func(file string) (bool, error) {
		t.Fatalf("configured installer mode should not prompt, got %q", file)
		return false, nil
	}

	result, err := runner.Run(path, Options{IsGUI: true, InstallMode: InstallModeInstaller})
	assert.NoErr(t, err)
	assert.Eq(t, path, launcher.path)
	assert.Eq(t, InstallerKindEXE, launcher.kind)
	assert.True(t, result.IsGUI)
	assert.Eq(t, InstallModeInstaller, result.InstallMode)
}

func TestRunExtractAllDoesNotPromptForDetectedInstaller(t *testing.T) {
	assetURL := "https://example.com/PicoClaw-Setup.exe"
	svc := NewService()
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return fakeInstallExtractor{file: ExtractedFile{
			Name:        "PicoClaw-Setup.exe",
			ArchiveName: "PicoClaw-Setup.exe",
			Extract: func(to string) error {
				if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
					return err
				}
				return os.WriteFile(to, []byte("installer"), 0o755)
			},
		}}
	}
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return &fakeChooser{name: pattern}, nil
	}
	svc.NoVerifierFactory = func() Verifier {
		return &fakeVerifier{}
	}

	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer")),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stderr = io.Discard
	var prompted bool
	runner.ConfirmLaunchInstaller = func(file string) (bool, error) {
		prompted = true
		return true, nil
	}

	result, err := runner.Run(assetURL, Options{All: true, Output: t.TempDir()})
	if err != nil {
		t.Fatalf("run extract-all installer-looking file: %v", err)
	}
	if prompted {
		t.Fatal("expected extract-all to skip installer launch prompt")
	}
	if result.IsGUI || result.InstallMode == InstallModeInstaller {
		t.Fatalf("expected extracted file result, got %#v", result)
	}
	if len(result.ExtractedFiles) != 1 {
		t.Fatalf("expected extracted file, got %#v", result.ExtractedFiles)
	}
}

func TestRunTemplateFinderResolvesChecksumVarsForVerifier(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	outputDir := t.TempDir()
	origDownloadGetWithOptions := downloadGetWithOptions
	origHTTPDo := httpDo
	defer func() {
		downloadGetWithOptions = origDownloadGetWithOptions
		httpDo = origHTTPDo
	}()

	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		assert.Eq(t, assetURL, url)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("binary-data")),
		}, nil
	}
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		_ = client
		assert.Eq(t, "https://example.com/1.2.3/manifest.json", req.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"platforms":{"win32-x64":{"checksum":"abc"}}}`)),
		}, nil
	}

	svc := NewService()
	svc.TemplateGetterFactory = func(opts Options) urltemplate.HTTPGetter {
		return fakeHTTPGetterFunc(func(url string) (*http.Response, error) {
			assert.Eq(t, "https://example.com/latest", url)
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("1.2.3")),
			}, nil
		})
	}
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	svc.SystemDetectorFactory = func(goos, goarch string) (Detector, error) {
		assert.Eq(t, "windows", goos)
		assert.Eq(t, "amd64", goarch)
		return &fakeDetector{name: assetURL}, nil
	}
	var verifierValue string
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		verifierValue = expected
		return &fakeVerifier{}, nil
	}
	svc.BinaryChooserFactory = func(tool string) any {
		return &fakeChooser{name: tool}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return fakeInstallExtractor{file: ExtractedFile{
			Name:        "claude.exe",
			ArchiveName: "claude.exe",
			Extract: func(to string) error {
				return os.WriteFile(to, []byte("binary-data"), 0o755)
			},
		}}
	}

	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard

	result, err := runner.Run("template:claude", Options{
		System: "windows/amd64",
		Output: outputDir,
		URLTemplate: URLTemplateOptions{
			LatestURL:           "https://example.com/latest",
			URLTemplate:         "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:               map[string]string{"windows": "win32"},
			ArchMap:             map[string]string{"amd64": "x64"},
			ExtMap:              map[string]string{"windows": ".exe"},
			ChecksumURLTemplate: "https://example.com/{version}/manifest.json",
			ChecksumFormat:      "json",
			ChecksumJSONPath:    "platforms.{os}-{arch}.checksum",
		},
	})
	if err != nil {
		t.Fatalf("run template install: %v", err)
	}

	assert.Eq(t, "abc", verifierValue)
	assert.Eq(t, assetURL, result.URL)
	assert.Eq(t, "1.2.3", result.Version)
	assert.Eq(t, []string{filepath.Join(outputDir, "claude.exe")}, result.ExtractedFiles)
}

func TestRunTreatsConfiguredOutputAsDirectoryWhenMissing(t *testing.T) {
	assetURL := "https://example.com/mutagen_linux_amd64_v0.18.1.tar.gz"
	origDownloadGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origDownloadGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		assert.Eq(t, assetURL, url)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("archive-data")),
		}, nil
	}

	svc := NewService()
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	svc.NoVerifierFactory = func() Verifier {
		return &fakeVerifier{}
	}
	svc.BinaryChooserFactory = func(tool string) any {
		return &fakeChooser{name: "mutagen"}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return fakeInstallExtractor{file: ExtractedFile{
			Name:        "mutagen",
			ArchiveName: "mutagen",
			mode:        0o755,
			Extract: func(to string) error {
				if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
					return err
				}
				return os.WriteFile(to, []byte("binary-data"), 0o755)
			},
		}}
	}

	outputDir := filepath.Join(t.TempDir(), "bin")
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard

	result, err := runner.Run(assetURL, Options{Output: outputDir})
	if err != nil {
		t.Fatalf("run install: %v", err)
	}

	want := filepath.Join(outputDir, "mutagen")
	assert.Eq(t, []string{want}, result.ExtractedFiles)
	data, err := os.ReadFile(want)
	assert.NoErr(t, err)
	assert.Eq(t, "binary-data", string(data))
	info, err := os.Stat(outputDir)
	assert.NoErr(t, err)
	assert.True(t, info.IsDir())
}

func TestRunAssetRunsVerifiedDownloadedAsset(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	origDownloadGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origDownloadGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		assert.Eq(t, assetURL, url)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer-data")),
		}, nil
	}

	svc := newRunAssetTestService(t, assetURL)
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard

	var launchedPath string
	var launchedArgs []string
	runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
		launchedPath = path
		launchedArgs = append([]string(nil), args...)
		data, err := os.ReadFile(path)
		assert.NoErr(t, err)
		assert.Eq(t, "installer-data", string(data))
		assert.NotNil(t, stdout)
		assert.NotNil(t, stderr)
		return nil
	}

	result, err := runner.Run("template:claude", Options{
		System:   "windows/amd64",
		CacheDir: t.TempDir(),
		Verify:   "abc",
		URLTemplate: URLTemplateOptions{
			URLTemplate:   "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			InstallAction: InstallActionRunAsset,
			InstallArgs:   []string{"install", "latest"},
		},
		Tag: "1.2.3",
	})
	if err != nil {
		t.Fatalf("run asset: %v", err)
	}

	assert.Contains(t, filepath.Base(launchedPath), "claude")
	assert.Eq(t, []string{"install", "latest"}, launchedArgs)
	assert.Eq(t, InstallModeRunAsset, result.InstallMode)
	assert.Eq(t, "1.2.3", result.Version)
	assert.Eq(t, assetURL, result.URL)
}

func TestRunAssetDownloadOnlyDoesNotRunAsset(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	origDownloadGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origDownloadGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		assert.Eq(t, assetURL, url)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer-data")),
		}, nil
	}

	svc := newRunAssetTestService(t, assetURL)
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
		t.Fatal("asset runner should not be called for download-only")
		return nil
	}
	outputDir := t.TempDir()

	result, err := runner.Run("template:claude", Options{
		System:       "windows/amd64",
		Output:       outputDir,
		DownloadOnly: true,
		CacheDir:     t.TempDir(),
		Verify:       "abc",
		URLTemplate: URLTemplateOptions{
			URLTemplate:   "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			InstallAction: InstallActionRunAsset,
			InstallArgs:   []string{"install", "latest"},
		},
		Tag: "1.2.3",
	})
	if err != nil {
		t.Fatalf("download run asset: %v", err)
	}

	wantPath := filepath.Join(outputDir, "claude.exe")
	assert.Eq(t, []string{wantPath}, result.ExtractedFiles)
	data, err := os.ReadFile(wantPath)
	assert.NoErr(t, err)
	assert.Eq(t, "installer-data", string(data))
	assert.Eq(t, "", result.InstallMode)
}

func TestRunAssetDoesNotRunWhenChecksumFails(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	origDownloadGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origDownloadGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer-data")),
		}, nil
	}

	svc := newRunAssetTestService(t, assetURL)
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		return failingVerifier{}, nil
	}
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
		t.Fatal("asset runner should not be called after checksum failure")
		return nil
	}

	_, err := runner.Run("template:claude", Options{
		System:   "windows/amd64",
		CacheDir: t.TempDir(),
		Verify:   "abc",
		URLTemplate: URLTemplateOptions{
			URLTemplate:   "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			InstallAction: InstallActionRunAsset,
		},
		Tag: "1.2.3",
	})

	if err == nil || !strings.Contains(err.Error(), "checksum failed") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
}

func TestRunAssetRequiresChecksumSource(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	svc := newRunAssetTestService(t, assetURL)
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
		t.Fatal("asset runner should not be called without checksum source")
		return nil
	}

	_, err := runner.Run("template:claude", Options{
		System:   "windows/amd64",
		CacheDir: t.TempDir(),
		URLTemplate: URLTemplateOptions{
			URLTemplate:   "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			InstallAction: InstallActionRunAsset,
		},
		Tag: "1.2.3",
	})

	if err == nil || !strings.Contains(err.Error(), "requires checksum") {
		t.Fatalf("expected checksum source error, got %v", err)
	}
}

func newRunAssetTestService(t *testing.T, assetURL string) *Service {
	t.Helper()
	svc := NewService()
	svc.SystemDetectorFactory = func(goos, goarch string) (Detector, error) {
		assert.Eq(t, "windows", goos)
		assert.Eq(t, "amd64", goarch)
		return &fakeDetector{name: assetURL}, nil
	}
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		assert.Eq(t, "abc", expected)
		return &fakeVerifier{}, nil
	}
	svc.DownloadOnlyExtractorFactory = func(name string) any {
		return NewDownloadOnlyExtractor(name)
	}
	return svc
}

type failingVerifier struct{}

func (failingVerifier) Verify([]byte) error {
	return errors.New("checksum failed")
}

type fakeInstallExtractor struct {
	file ExtractedFile
}

func (f fakeInstallExtractor) Extract([]byte, bool) (ExtractedFile, []ExtractedFile, error) {
	return f.file, nil, nil
}

type fakeDirectAllExtractor struct {
	extractCalled bool
	files         []string
	strip         int
}

func (f *fakeDirectAllExtractor) Extract([]byte, bool) (ExtractedFile, []ExtractedFile, error) {
	f.extractCalled = true
	return ExtractedFile{}, nil, nil
}

func (f *fakeDirectAllExtractor) ExtractAllTo([]byte, string) ([]string, error) {
	return f.files, nil
}

func (f *fakeDirectAllExtractor) ExtractAllToWithOptions(data []byte, output string, opts ArchiveExtractOptions) ([]string, error) {
	f.strip = opts.StripComponents
	return f.ExtractAllTo(data, output)
}

func TestRunExtractAllUsesDirectExtractorWithoutListing(t *testing.T) {
	assetURL := "https://example.com/tool.7z"
	outputDir := t.TempDir()
	direct := &fakeDirectAllExtractor{
		files: []string{filepath.Join(outputDir, "$PLUGINSDIR", "nsDialogs.dll")},
	}

	svc := NewService()
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return direct
	}
	svc.System7zPathResolver = func(configured string) string {
		return ""
	}
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return NewFileChooser(pattern)
	}
	svc.NoVerifierFactory = func() Verifier {
		return &fakeVerifier{}
	}

	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer")),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stderr = io.Discard
	result, err := runner.Run(assetURL, Options{All: true, Output: outputDir})
	if err != nil {
		t.Fatalf("run extract-all: %v", err)
	}

	if direct.extractCalled {
		t.Fatal("expected direct extract-all to skip Extract")
	}
	if len(result.ExtractedFiles) != 1 {
		t.Fatalf("expected extracted files, got %#v", result.ExtractedFiles)
	}
}

func TestRunExtractAllPassesStripComponentsToDirectExtractor(t *testing.T) {
	assetURL := "https://example.com/ventoy.zip"
	outputDir := t.TempDir()
	direct := &fakeDirectAllExtractor{
		files: []string{filepath.Join(outputDir, "Ventoy2Disk.exe")},
	}

	svc := NewService()
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return direct
	}
	svc.System7zPathResolver = func(configured string) string {
		return ""
	}
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return NewFileChooser(pattern)
	}
	svc.NoVerifierFactory = func() Verifier {
		return &fakeVerifier{}
	}

	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("zip")),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stderr = io.Discard
	_, err := runner.Run(assetURL, Options{All: true, Output: outputDir, StripComponents: 1})
	if err != nil {
		t.Fatalf("run extract-all: %v", err)
	}

	assert.Eq(t, 1, direct.strip)
}

func TestRunExtractAllStripsComponentsFromZipArchive(t *testing.T) {
	assetURL := "https://example.com/ventoy.zip"
	archive := zipBytes(t, map[string]string{
		"ventoy-1.1.12/Ventoy2Disk.exe": "exe",
		"ventoy-1.1.12/boot/boot.img":   "boot",
	})
	outputDir := t.TempDir()

	svc := NewDefaultService(nil, nil)
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(archive)),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	result, err := runner.Run(assetURL, Options{All: true, Output: outputDir, StripComponents: 1})
	if err != nil {
		t.Fatalf("run extract-all: %v", err)
	}

	assert.Eq(t, 2, len(result.ExtractedFiles))
	data, err := os.ReadFile(filepath.Join(outputDir, "Ventoy2Disk.exe"))
	if err != nil {
		t.Fatalf("read stripped exe: %v", err)
	}
	assert.Eq(t, "exe", string(data))
	data, err = os.ReadFile(filepath.Join(outputDir, "boot", "boot.img"))
	if err != nil {
		t.Fatalf("read stripped boot file: %v", err)
	}
	assert.Eq(t, "boot", string(data))
	if _, err := os.Stat(filepath.Join(outputDir, "ventoy-1.1.12")); !os.IsNotExist(err) {
		t.Fatalf("expected stripped root directory to be absent, stat err=%v", err)
	}
}

func TestRunAutoExtractsMultipleWindowsExecutables(t *testing.T) {
	assetURL := "https://example.com/uv.zip"
	archive := zipBytes(t, map[string]string{
		"uv.exe":      "uv",
		"uvx.exe":     "uvx",
		"uvw.exe":     "uvw",
		"README.md":   "readme",
		"LICENSE.txt": "license",
	})
	outputDir := t.TempDir()

	svc := NewDefaultService(nil, nil)
	svc.SystemDetectorFactory = func(goos, goarch string) (Detector, error) {
		assert.Eq(t, "windows", goos)
		assert.Eq(t, "amd64", goarch)
		return &fakeDetector{name: assetURL}, nil
	}
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(archive)),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	result, err := runner.Run(assetURL, Options{
		Name:   "uv",
		Output: outputDir,
		System: "windows/amd64",
	})
	if err != nil {
		t.Fatalf("run install: %v", err)
	}

	gotFiles := append([]string(nil), result.ExtractedFiles...)
	wantFiles := []string{
		filepath.Join(outputDir, "uv.exe"),
		filepath.Join(outputDir, "uvx.exe"),
		filepath.Join(outputDir, "uvw.exe"),
	}
	sort.Strings(gotFiles)
	sort.Strings(wantFiles)
	assert.Eq(t, wantFiles, gotFiles)
	for _, file := range result.ExtractedFiles {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("expected extracted file %s: %v", file, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("expected README.md not to be auto-extracted, stat err=%v", err)
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0o644)
		w, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatalf("create zip file: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip file: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestRunQuietSuppressesInstallNotice(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "tool.exe")
	if err := os.WriteFile(source, []byte("tool"), 0o755); err != nil {
		t.Fatalf("write source: %v", err)
	}
	outputDir := filepath.Join(tmpDir, "bin")
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}

	var stderr bytes.Buffer
	var globalOut bytes.Buffer
	ccolor.SetOutput(&globalOut)
	defer ccolor.SetOutput(os.Stdout)

	runner := NewRunner(NewDefaultService(nil, nil))
	runner.Stderr = &stderr

	if _, err := runner.Run(source, Options{Quiet: true, DownloadOnly: true, Output: outputDir}); err != nil {
		t.Fatalf("run quiet install: %v", err)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("expected quiet stderr to be empty, got %q", got)
	}
	if got := globalOut.String(); got != "" {
		t.Fatalf("expected quiet global output to be empty, got %q", got)
	}
}

func TestRunWritesSuccessfulInstallOutputToStdout(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "tool.exe")
	if err := os.WriteFile(source, []byte("tool"), 0o755); err != nil {
		t.Fatalf("write source: %v", err)
	}
	outputDir := filepath.Join(tmpDir, "bin")
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(NewDefaultService(nil, nil))
	runner.Stdout = &stdout
	runner.Stderr = &stderr

	if _, err := runner.Run(source, Options{DownloadOnly: true, Output: outputDir}); err != nil {
		t.Fatalf("run install: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "Install") || !strings.Contains(got, "Asset") {
		t.Fatalf("expected successful install output on stdout, got %q", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("expected successful install stderr to be empty, got %q", got)
	}
}

func TestRunStopsWhenConfiguredAssetFilterMatchesNoCurrentReleaseAssets(t *testing.T) {
	svc := NewDefaultService(nil, nil)
	svc.GitHubGetterFactory = func(opts Options) sourcegithub.HTTPGetter {
		return HTTPGetterFunc(func(url string) (*http.Response, error) {
			if !strings.Contains(url, "/repos/Zxilly/go-size-analyzer/releases/latest") {
				t.Fatalf("unexpected GitHub API request %q", url)
			}
			body := `{"assets":[{"browser_download_url":"https://github.com/Zxilly/go-size-analyzer/releases/download/v1.12.5/go-size-analyzer_1.12.5_linux_amd64.tar.gz"}]}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})
	}

	downloadCalls := 0
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		downloadCalls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("old-asset")),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stderr = io.Discard
	runner.InstalledLoad = func() (map[string]string, map[string]string, error) {
		return map[string]string{}, map[string]string{
			"Zxilly/go-size-analyzer": "https://github.com/Zxilly/go-size-analyzer/releases/download/v1.12.4/go-size-analyzer_1.12.4_windows_amd64.zip",
		}, nil
	}

	_, err := runner.Run("Zxilly/go-size-analyzer", Options{
		System: "windows/amd64",
		Asset:  []string{"windows"},
	})
	if err == nil || !strings.Contains(err.Error(), "asset `windows` not found") {
		t.Fatalf("expected missing current asset error, got %v", err)
	}
	if downloadCalls != 0 {
		t.Fatalf("expected no download when current release asset does not match, got %d calls", downloadCalls)
	}
}

func TestResolveCandidateSelectsUniqueNameMatch(t *testing.T) {
	runner := &InstallRunner{Stderr: io.Discard}
	runner.Prompt = func(title, filterPrompt string, choices []string) (int, error) {
		t.Fatalf("expected --name to avoid prompt, got choices %#v", choices)
		return 0, nil
	}

	got, err := runner.resolveCandidate("gookit/greq", []string{
		"https://github.com/gookit/greq/releases/download/v0.6.0/gbench-v0.6.0-windows-amd64.zip",
		"https://github.com/gookit/greq/releases/download/v0.6.0/greq-v0.6.0-windows-amd64.zip",
	}, Options{Name: "gbench"}, "")

	assert.NoErr(t, err)
	assert.Eq(t, "https://github.com/gookit/greq/releases/download/v0.6.0/gbench-v0.6.0-windows-amd64.zip", got)
}

func TestResolveCandidateKeepsPromptWhenNameMatchIsAmbiguous(t *testing.T) {
	runner := &InstallRunner{Stderr: io.Discard}
	prompted := false
	runner.Prompt = func(title, filterPrompt string, choices []string) (int, error) {
		prompted = true
		assert.Eq(t, "Select package resource v0.6.0", title)
		assert.Eq(t, "Filter assets", filterPrompt)
		assert.Eq(t, []string{
			"gbench-v0.6.0-windows-amd64.zip",
			"gbench-lite-v0.6.0-windows-amd64.zip",
		}, choices)
		return 1, nil
	}

	got, err := runner.resolveCandidate("gookit/greq", []string{
		"https://github.com/gookit/greq/releases/download/v0.6.0/gbench-v0.6.0-windows-amd64.zip",
		"https://github.com/gookit/greq/releases/download/v0.6.0/gbench-lite-v0.6.0-windows-amd64.zip",
	}, Options{Name: "gbench"}, "v0.6.0")

	assert.NoErr(t, err)
	assert.True(t, prompted)
	assert.Eq(t, "https://github.com/gookit/greq/releases/download/v0.6.0/gbench-lite-v0.6.0-windows-amd64.zip", got)
}

func TestResolveExtractedFileUsesExtractedFilePromptTitle(t *testing.T) {
	runner := &InstallRunner{Stderr: io.Discard}
	prompted := false
	runner.Prompt = func(title, filterPrompt string, choices []string) (int, error) {
		prompted = true
		assert.Eq(t, "Select extracted file", title)
		assert.Eq(t, "Filter files", filterPrompt)
		assert.Eq(t, []string{"gsa.exe", "gsa-helper.exe", "all"}, choices)
		return 0, nil
	}

	selected, all, err := runner.resolveExtractedFile([]ExtractedFile{
		{ArchiveName: `gsa.exe`, Name: `gsa.exe`, mode: 0o666},
		{ArchiveName: `gsa-helper.exe`, Name: `gsa-helper.exe`, mode: 0o666},
	}, Options{System: "windows/amd64"})

	assert.NoErr(t, err)
	assert.True(t, prompted)
	assert.False(t, all)
	assert.Eq(t, `gsa.exe`, selected.ArchiveName)
}

func TestRunFallsBackToOlderSourceForgeVersionWhenAssetMissing(t *testing.T) {
	responses := map[string]string{
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/": `
<script>
net.sf.files = {
  "2.59": {"name":"2.59","full_path":"/Translations 2.x/2.59","type":"d"},
  "2.60": {"name":"2.60","full_path":"/Translations 2.x/2.60","type":"d"},
  "2.61": {"name":"2.61","full_path":"/Translations 2.x/2.61","type":"d"}
};
</script>`,
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.61/": `
<script>
net.sf.files = {
  "Spanish.zip": {
    "name":"Spanish.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.61/Spanish.zip",
    "full_path":"/Translations 2.x/2.61/Spanish.zip",
    "type":"f"
  }
};
</script>`,
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.60/": `
<script>
net.sf.files = {
  "German.zip": {
    "name":"German.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.60/German.zip",
    "full_path":"/Translations 2.x/2.60/German.zip",
    "type":"f"
  }
};
</script>`,
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.59/": `
<script>
net.sf.files = {
  "Ukrainian.zip": {
    "name":"Ukrainian.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.59/Ukrainian.zip",
    "full_path":"/Translations 2.x/2.59/Ukrainian.zip",
    "type":"f"
  }
};
</script>`,
	}
	var sourceForgeRequests []string
	svc := NewDefaultService(nil, nil)
	svc.SourceForgeGetterFactory = func(opts Options) sourcesf.HTTPGetter {
		return HTTPGetterFunc(func(url string) (*http.Response, error) {
			sourceForgeRequests = append(sourceForgeRequests, url)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responses[url])),
			}, nil
		})
	}

	var downloadedURL string
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		downloadedURL = url
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("translation")),
		}, nil
	}

	outputDir := t.TempDir()
	runner := NewRunner(svc)
	runner.Stderr = io.Discard
	result, err := runner.Run("sourceforge:keepass/Translations 2.x", Options{
		FallbackVersions: 10,
		Asset:            []string{"Ukrainian", "zip"},
		DownloadOnly:     true,
		Output:           outputDir,
	})

	if err != nil {
		t.Fatalf("run sourceforge fallback: %v", err)
	}
	wantURL := "https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.59/Ukrainian.zip"
	if result.URL != wantURL || downloadedURL != wantURL {
		t.Fatalf("expected fallback URL %q, got result=%q downloaded=%q", wantURL, result.URL, downloadedURL)
	}
	assertRequests := []string{
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/",
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.61/",
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/",
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.60/",
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.59/",
	}
	if strings.Join(sourceForgeRequests, "\n") != strings.Join(assertRequests, "\n") {
		t.Fatalf("unexpected sourceforge requests:\n%v", strings.Join(sourceForgeRequests, "\n"))
	}
}

func TestDefaultConfirmLaunchInstallerTreatsBlankLineAsCancel(t *testing.T) {
	origStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = origStdin
		_ = reader.Close()
	}()
	if _, err := writer.WriteString("\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}

	confirmed, err := defaultConfirmLaunchInstaller("Clash.Verge_2.4.7_x64-setup.exe")
	if err != nil {
		t.Fatalf("expected blank line to cancel without error, got %v", err)
	}
	if confirmed {
		t.Fatal("expected blank line to cancel installer launch")
	}
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

func TestOutputPathUsesHeuristicExecutableRename(t *testing.T) {
	file := ExtractedFile{Name: "chlog-windows-amd64.exe", mode: 0o666}
	got, err := outputPath(file, "", false, "", false)
	if err != nil {
		t.Fatalf("outputPath(): %v", err)
	}
	if got != "chlog.exe" {
		t.Fatalf("expected heuristic output name chlog.exe, got %q", got)
	}
}

func TestOutputPathUsesPreferredNameForExecutable(t *testing.T) {
	file := ExtractedFile{Name: "chlog-windows-amd64.exe", mode: 0o666}
	got, err := outputPath(file, "", false, "chlog", false)
	if err != nil {
		t.Fatalf("outputPath(): %v", err)
	}
	if got != "chlog.exe" {
		t.Fatalf("expected preferred output name chlog.exe, got %q", got)
	}
}

func TestOutputPathUsesPreferredNameForVersionedPortableExecutable(t *testing.T) {
	file := ExtractedFile{Name: "Alacritty-v0.17.0-portable.exe", mode: 0o666}
	got, err := outputPath(file, "", false, "alacritty", false)
	if err != nil {
		t.Fatalf("outputPath(): %v", err)
	}
	if got != "alacritty.exe" {
		t.Fatalf("expected preferred output name alacritty.exe, got %q", got)
	}
}

func TestOutputPathKeepsExecutableNameWhenPreferredNameDoesNotMatchPlatformSuffix(t *testing.T) {
	file := ExtractedFile{Name: "bd.exe", mode: 0o666}
	got, err := outputPath(file, "", false, "beads", false)
	if err != nil {
		t.Fatalf("outputPath(): %v", err)
	}
	if got != "bd.exe" {
		t.Fatalf("expected executable name bd.exe to be preserved, got %q", got)
	}
}

func TestOutputPathUsesPreferredNameWithExplicitExtension(t *testing.T) {
	file := ExtractedFile{Name: "chlog-windows-amd64.exe", mode: 0o666}
	got, err := outputPath(file, "", false, "custom-name.exe", false)
	if err != nil {
		t.Fatalf("outputPath(): %v", err)
	}
	if got != "custom-name.exe" {
		t.Fatalf("expected preferred explicit output name custom-name.exe, got %q", got)
	}
}

func TestOutputPathKeepsExplicitFileOutput(t *testing.T) {
	file := ExtractedFile{Name: "chlog-windows-amd64.exe", mode: 0o666}
	outputFile := filepath.Join(t.TempDir(), "custom-tool")
	got, err := outputPath(file, outputFile, false, "", true)
	if err != nil {
		t.Fatalf("outputPath(): %v", err)
	}
	if got != outputFile {
		t.Fatalf("expected explicit file output %q, got %q", outputFile, got)
	}
}

func TestOutputPathKeepsArchiveDirectoriesForExtractAll(t *testing.T) {
	file := ExtractedFile{Name: "Far/7-ZipEng.hlf", mode: 0o644}
	got, err := outputPath(file, "dist", true, "", false)
	if err != nil {
		t.Fatalf("outputPath(): %v", err)
	}
	want := filepath.Join("dist", "Far", "7-ZipEng.hlf")
	if got != want {
		t.Fatalf("expected extract-all output path %q, got %q", want, got)
	}
}

func TestOutputPathAppliesRenameFileForExtractAll(t *testing.T) {
	file := ExtractedFile{Name: "codex-x86_64-pc-windows-msvc.exe", mode: 0o666}
	got, err := outputPath(file, "bin", true, "", false, map[string]string{
		"codex-x86_64-pc-windows-msvc.exe": "codex.exe",
	})
	if err != nil {
		t.Fatalf("outputPath(): %v", err)
	}
	want := filepath.Join("bin", "codex.exe")
	if got != want {
		t.Fatalf("expected renamed extract-all output path %q, got %q", want, got)
	}
}

func TestOutputPathRejectsUnsafeRenameFileTarget(t *testing.T) {
	file := ExtractedFile{Name: "codex.exe", mode: 0o666}
	_, err := outputPath(file, "bin", true, "", false, map[string]string{
		"codex.exe": "../codex.exe",
	})
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("expected unsafe archive path error, got %v", err)
	}
}

func TestOutputPathRejectsArchivePathTraversalForExtractAll(t *testing.T) {
	file := ExtractedFile{Name: "../evil.exe", mode: 0o644}
	if _, err := outputPath(file, "dist", true, "", false); err == nil {
		t.Fatal("expected archive path traversal to be rejected")
	}
}

func TestAutoSelectExtractedFileByArch(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `arm64\WinDirStat.exe`, Name: `arm64\WinDirStat.exe`, mode: 0o666},
		{ArchiveName: `x86\WinDirStat.exe`, Name: `x86\WinDirStat.exe`, mode: 0o666},
		{ArchiveName: `x64\WinDirStat.exe`, Name: `x64\WinDirStat.exe`, mode: 0o666},
	}

	selected, ok := autoSelectExtractedFile(candidates, "windows", "amd64")
	if !ok {
		t.Fatal("expected auto selection for amd64 candidates")
	}
	if selected.ArchiveName != `x64\WinDirStat.exe` {
		t.Fatalf("expected x64 executable to be selected, got %q", selected.ArchiveName)
	}
}

func TestAutoSelectExtractedFilePicksOnlyWindowsExe(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `LICENSE`, Name: `LICENSE`, mode: 0o666},
		{ArchiveName: `gsa.exe`, Name: `gsa.exe`, mode: 0o666},
	}

	selected, ok := autoSelectExtractedFile(candidates, "windows", "amd64")
	if !ok {
		t.Fatal("expected auto selection for the only Windows executable")
	}
	if selected.ArchiveName != `gsa.exe` {
		t.Fatalf("expected gsa.exe to be selected, got %q", selected.ArchiveName)
	}
}

func TestAutoSelectExtractedFileKeepsPromptForMultipleWindowsExe(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `gsa.exe`, Name: `gsa.exe`, mode: 0o666},
		{ArchiveName: `gsa-helper.exe`, Name: `gsa-helper.exe`, mode: 0o666},
	}

	_, ok := autoSelectExtractedFile(candidates, "windows", "amd64")
	if ok {
		t.Fatal("expected multiple Windows executables to keep prompt fallback")
	}
}

func TestResolveExtractedFileUsesExplicitSystemForWindowsExeSelection(t *testing.T) {
	runner := &InstallRunner{Stderr: io.Discard}
	candidates := []ExtractedFile{
		{ArchiveName: `LICENSE`, Name: `LICENSE`, mode: 0o666},
		{ArchiveName: `gsa.exe`, Name: `gsa.exe`, mode: 0o666},
	}

	selected, all, err := runner.resolveExtractedFile(candidates, Options{System: "windows/amd64"})
	if err != nil {
		t.Fatalf("resolve extracted file: %v", err)
	}
	if all {
		t.Fatal("expected single file selection, got extract-all")
	}
	if selected.ArchiveName != `gsa.exe` {
		t.Fatalf("expected gsa.exe to be selected, got %q", selected.ArchiveName)
	}
}

func TestAutoSelectExtractedFileKeepsPromptWhenAmbiguous(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `bin\tool.exe`, Name: `bin\tool.exe`, mode: 0o666},
		{ArchiveName: `tools\tool.exe`, Name: `tools\tool.exe`, mode: 0o666},
	}

	_, ok := autoSelectExtractedFile(candidates, "windows", "amd64")
	if ok {
		t.Fatal("expected ambiguous candidates to keep prompt fallback")
	}
}

func TestAutoExtractCurrentPlatformExecutablesFiltersOtherPlatformExecutables(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `x86_64\uv.exe`, Name: `x86_64\uv.exe`, mode: 0o666},
		{ArchiveName: `x86\uv.exe`, Name: `x86\uv.exe`, mode: 0o666},
		{ArchiveName: `linux\uv`, Name: `linux\uv`, mode: 0o755},
	}

	selected, ok := autoExtractCurrentPlatformExecutables(candidates, Options{System: "windows/amd64"})
	if !ok {
		t.Fatal("expected current-platform executable selection")
	}
	assert.Eq(t, 1, len(selected))
	assert.Eq(t, `x86_64\uv.exe`, selected[0].ArchiveName)
}
