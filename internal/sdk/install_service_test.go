package sdk

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

func TestServiceInstallExactVersionRecordsSDK(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "go.zip")
	writeSDKZip(t, archivePath, map[string]string{
		"go/bin/go": "go",
	})
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	cfg := testSDKConfig(root)
	now := time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC)
	svc := Service{
		Config:     cfg,
		Store:      store,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        func() time.Time { return now },
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://example.com/go1.21.1.linux-amd64.zip", req.URL)
			return DownloadResult{Path: archivePath, Size: 123}, nil
		},
	}

	result, err := svc.Install(context.Background(), "go@1.21.1", InstallOptions{})
	if err != nil {
		t.Fatalf("install sdk: %v", err)
	}

	assert.Eq(t, "go", result.Name)
	assert.Eq(t, "1.21.1", result.Version)
	data, err := os.ReadFile(filepath.Join(result.Path, "bin", "go"))
	if err != nil {
		t.Fatalf("read installed go: %v", err)
	}
	assert.Eq(t, "go", string(data))

	entries, err := store.List("go")
	if err != nil {
		t.Fatalf("list store: %v", err)
	}
	assert.Eq(t, 1, len(entries))
	assert.Eq(t, result.Path, entries[0].Path)
	assert.Eq(t, now, entries[0].InstalledAt)
}

func TestServiceInstallUsesIndexForPrefixVersion(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "go.zip")
	writeSDKZip(t, archivePath, map[string]string{"go/bin/go": "go"})
	cfg := testSDKConfig(root)
	svc := Service{
		Config:     cfg,
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://mirror/go1.21.13.linux-amd64.zip", req.URL)
			return DownloadResult{Path: archivePath}, nil
		},
	}
	err := svc.IndexCache.Save(Index{
		Schema:    1,
		SDK:       "go",
		SourceURL: "https://example.com/golang/",
		Items: []IndexItem{
			{Version: "1.21.1", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror/go1.21.1.linux-amd64.zip", Filename: "go1.21.1.linux-amd64.zip"}}},
			{Version: "1.21.13", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror/go1.21.13.linux-amd64.zip", Filename: "go1.21.13.linux-amd64.zip"}}},
		},
	})
	if err != nil {
		t.Fatalf("save index: %v", err)
	}

	result, err := svc.Install(context.Background(), "go:1.21", InstallOptions{})
	if err != nil {
		t.Fatalf("install sdk: %v", err)
	}

	assert.Eq(t, "1.21.13", result.Version)
}

func TestServiceInstallPassesDownloadProgress(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "go.zip")
	writeSDKZip(t, archivePath, map[string]string{"go/bin/go": "go"})
	cfg := testSDKConfig(root)
	progressCalled := false
	var startTarget string
	var startVersion string
	var startHost string
	svc := Service{
		Config:     cfg,
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.NotNil(t, req.Progress)
			progressCalled = true
			return DownloadResult{Path: archivePath}, nil
		},
	}
	if err := svc.IndexCache.Save(Index{
		Schema:    1,
		SDK:       "go",
		SourceURL: "https://example.com/golang/",
		Items: []IndexItem{{
			Version: "1.21.1",
			Stable:  true,
			Files:   []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror/go1.21.1.linux-amd64.zip", Filename: "go1.21.1.linux-amd64.zip"}},
		}},
	}); err != nil {
		t.Fatalf("save index: %v", err)
	}

	_, err := svc.Install(context.Background(), "go:1.21", InstallOptions{
		Progress: func(size int64) io.Writer { return io.Discard },
		OnStart: func(target string, version string, host string) {
			startTarget = target
			startVersion = version
			startHost = host
		},
	})
	if err != nil {
		t.Fatalf("install sdk: %v", err)
	}

	assert.True(t, progressCalled)
	assert.Eq(t, "go:1.21", startTarget)
	assert.Eq(t, "1.21.1", startVersion)
	assert.Eq(t, "mirror", startHost)
}

func TestServiceInstallUsesIndexForConfiguredMirror(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "go.zip")
	writeSDKZip(t, archivePath, map[string]string{"go/bin/go": "go"})
	cfg := testSDKConfig(root)
	goSDK := cfg.SDK["go"]
	goSDK.IndexURL = stringPtr("https://mirror-b.example.com/golang/")
	cfg.SDK["go"] = goSDK
	svc := Service{
		Config:     cfg,
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://mirror-b/go1.22.0.linux-amd64.zip", req.URL)
			return DownloadResult{Path: archivePath}, nil
		},
	}
	if err := svc.IndexCache.Save(Index{
		Schema:    1,
		SDK:       "go",
		SourceURL: "https://mirror-a.example.com/golang/",
		Items: []IndexItem{
			{Version: "1.21.1", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror-a/go1.21.1.linux-amd64.zip", Filename: "go1.21.1.linux-amd64.zip"}}},
		},
	}); err != nil {
		t.Fatalf("save first index: %v", err)
	}
	if err := svc.IndexCache.Save(Index{
		Schema:    1,
		SDK:       "go",
		SourceURL: "https://mirror-b.example.com/golang/",
		Items: []IndexItem{
			{Version: "1.22.0", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror-b/go1.22.0.linux-amd64.zip", Filename: "go1.22.0.linux-amd64.zip"}}},
		},
	}); err != nil {
		t.Fatalf("save second index: %v", err)
	}

	result, err := svc.Install(context.Background(), "go@latest", InstallOptions{})
	if err != nil {
		t.Fatalf("install sdk: %v", err)
	}

	assert.Eq(t, "1.22.0", result.Version)
}

func TestServiceInstallRejectsExistingPathUnlessForce(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "go.zip")
	writeSDKZip(t, archivePath, map[string]string{"go/bin/go": "go"})
	cfg := testSDKConfig(root)
	existing := filepath.Join(root, "sdks", "gosdk", "go1.21.1")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("create existing path: %v", err)
	}
	svc := Service{
		Config:     cfg,
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			return DownloadResult{Path: archivePath}, nil
		},
	}

	_, err := svc.Install(context.Background(), "go@1.21.1", InstallOptions{})
	assert.Err(t, err)

	_, err = svc.Install(context.Background(), "go@1.21.1", InstallOptions{Force: true})
	if err != nil {
		t.Fatalf("force install sdk: %v", err)
	}
}

func TestServiceInstallFromLocalHTTPServer(t *testing.T) {
	root := t.TempDir()
	archive := sdkZipBytes(t, map[string]string{"go/bin/go": "go"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/golang/":
			_, _ = io.WriteString(w, `<a href="go1.21.1.linux-amd64.zip">go</a>`)
		case "/golang/go1.21.1.linux-amd64.zip":
			w.Header().Set("Content-Length", stringLen(archive))
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := testSDKConfig(root)
	goSDK := cfg.SDK["go"]
	goSDK.URLTemplate = stringPtr(server.URL + "/golang/go{version}.{os}-{arch}.{ext}")
	goSDK.IndexURL = stringPtr(server.URL + "/golang/")
	cfg.SDK["go"] = goSDK
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	svc := Service{
		Config:     cfg,
		Store:      store,
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        func() time.Time { return time.Date(2026, 5, 17, 16, 0, 0, 0, time.UTC) },
	}

	result, err := svc.Install(context.Background(), "go@1.21.1", InstallOptions{})
	assert.NoErr(t, err)
	assert.Eq(t, "1.21.1", result.Version)

	data, err := os.ReadFile(filepath.Join(result.Path, "bin", "go"))
	assert.NoErr(t, err)
	assert.Eq(t, "go", string(data))
	entries, err := store.List("go")
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(entries))
	assert.Eq(t, result.Path, entries[0].Path)
}

func TestServiceInstallResumesArchiveDownload(t *testing.T) {
	root := t.TempDir()
	archive := sdkZipBytes(t, map[string]string{"go/bin/go": "go"})
	var downloadURL string
	var rangeHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("ETag", `"sdk-zip"`)
			w.Header().Set("Content-Length", stringLen(archive))
			return
		}
		if r.URL.Path != "/golang/go1.21.1.linux-amd64.zip" {
			http.NotFound(w, r)
			return
		}
		rangeHeader = r.Header.Get("Range")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"sdk-zip"`)
		if rangeHeader == "" {
			w.Header().Set("Content-Length", stringLen(archive))
			_, _ = w.Write(archive)
			return
		}
		start := len(archive) / 2
		w.Header().Set("Content-Length", stringLen(archive[start:]))
		w.Header().Set("Content-Range", "bytes "+intString(start)+"-"+intString(len(archive)-1)+"/"+intString(len(archive)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(archive[start:])
	}))
	defer server.Close()
	downloadURL = server.URL + "/golang/go1.21.1.linux-amd64.zip"

	cfg := testSDKConfig(root)
	goSDK := cfg.SDK["go"]
	goSDK.URLTemplate = stringPtr(server.URL + "/golang/go{version}.{os}-{arch}.{ext}")
	cfg.SDK["go"] = goSDK
	svc := Service{
		Config:     cfg,
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}
	req := DownloadRequest{
		URL:      downloadURL,
		CacheDir: svc.cacheDir(),
		SDK:      "go",
		Version:  "1.21.1",
		Filename: "go1.21.1.linux-amd64.zip",
	}
	partPath := sdkDownloadPartPath(req)
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		t.Fatalf("mkdir part dir: %v", err)
	}
	if err := os.WriteFile(partPath, archive[:len(archive)/2], 0o644); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := saveDownloadMeta(sdkDownloadMetaPath(req), downloadMeta{
		Schema:   1,
		URL:      req.URL,
		Filename: req.Filename,
		Size:     int64(len(archive)),
		ETag:     `"sdk-zip"`,
	}); err != nil {
		t.Fatalf("save download meta: %v", err)
	}

	result, err := svc.Install(context.Background(), "go@1.21.1", InstallOptions{})
	assert.NoErr(t, err)
	assert.True(t, result.Resumed)
	assert.Eq(t, "bytes="+intString(len(archive)/2)+"-", rangeHeader)
	data, err := os.ReadFile(filepath.Join(result.Path, "bin", "go"))
	assert.NoErr(t, err)
	assert.Eq(t, "go", string(data))
}
