package sdk

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
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

func TestServiceRemoveDeletesSafePathAndRecord(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	target := filepath.Join(root, "sdks", "gosdk", "go1.21.1")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	if err := store.Record(InstalledEntry{Name: "go", Version: "1.21.1", Path: target}); err != nil {
		t.Fatalf("record sdk: %v", err)
	}
	svc := Service{Config: cfg, Store: store, GOOS: "linux", GOARCH: "amd64"}

	result, err := svc.Remove("go@1.21.1")
	if err != nil {
		t.Fatalf("remove sdk: %v", err)
	}

	assert.Eq(t, target, result.Path)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected target removed, stat err=%v", err)
	}
	entries, err := store.List("go")
	if err != nil {
		t.Fatalf("list store: %v", err)
	}
	assert.Eq(t, 0, len(entries))
}

func TestServiceRemoveRejectsUnsafePath(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside: %v", err)
	}
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	if err := store.Record(InstalledEntry{Name: "go", Version: "1.21.1", Path: outside}); err != nil {
		t.Fatalf("record sdk: %v", err)
	}
	svc := Service{Config: cfg, Store: store, GOOS: "linux", GOARCH: "amd64"}

	_, err := svc.Remove("go@1.21.1")
	assert.Err(t, err)
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("expected unsafe path to remain: %v", statErr)
	}
}

func TestServiceRefreshShowListAndClearIndex(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<a href="go1.21.1.linux-amd64.zip">go</a>`)
	}))
	defer server.Close()
	cfg := testSDKConfig(root)
	goSDK := cfg.SDK["go"]
	goSDK.IndexURL = stringPtr(server.URL + "/golang/")
	cfg.SDK["go"] = goSDK
	svc := Service{
		Config:     cfg,
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        func() time.Time { return time.Date(2026, 5, 17, 15, 0, 0, 0, time.UTC) },
	}

	index, err := svc.RefreshIndex(context.Background(), "go")
	if err != nil {
		t.Fatalf("refresh index: %v", err)
	}
	assert.Eq(t, "go", index.SDK)
	assert.Eq(t, "1.21.1", index.Items[0].Version)

	shown, err := svc.ShowIndex("go")
	if err != nil {
		t.Fatalf("show index: %v", err)
	}
	assert.Eq(t, index.Items[0].Version, shown.Items[0].Version)

	infos, err := svc.ListIndexes()
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	assert.Eq(t, 1, len(infos))
	assert.Eq(t, "go", infos[0].SDK)

	if err := svc.ClearIndex("go"); err != nil {
		t.Fatalf("clear index: %v", err)
	}
	_, err = svc.ShowIndex("go")
	assert.Err(t, err)
}

func TestServiceSearchIndexMatchesKeywordsAndExcludes(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}
	err := svc.IndexCache.Save(Index{
		Schema: 1,
		SDK:    "go",
		Items: []IndexItem{
			{Version: "1.22.0", Stable: true, Files: []IndexFile{
				{OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "go1.22.0.linux-amd64.tar.gz", URL: "https://example.com/go1.22.0.linux-amd64.tar.gz"},
				{OS: "windows", Arch: "amd64", Ext: "zip", Filename: "go1.22.0.windows-amd64.zip", URL: "https://example.com/go1.22.0.windows-amd64.zip"},
			}},
			{Version: "1.22.1-rc.1", Stable: false, Files: []IndexFile{
				{OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "go1.22.1-rc.1.linux-amd64.tar.gz", URL: "https://example.com/go1.22.1-rc.1.linux-amd64.tar.gz"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("save index: %v", err)
	}

	results, err := svc.SearchIndex("go", SearchOptions{Keywords: []string{"1.22 amd64", "^windows ^rc"}, Number: 20})
	if err != nil {
		t.Fatalf("search index: %v", err)
	}

	assert.Eq(t, 1, len(results))
	assert.Eq(t, "go", results[0].SDK)
	assert.Eq(t, "1.22.0", results[0].Version)
	assert.True(t, results[0].Stable)
	assert.Eq(t, "linux", results[0].OS)
	assert.Eq(t, "amd64", results[0].Arch)
	assert.Eq(t, "go1.22.0.linux-amd64.tar.gz", results[0].Filename)
}

func TestServiceSearchIndexLimitsResults(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}
	err := svc.IndexCache.Save(Index{
		Schema: 1,
		SDK:    "go",
		Items: []IndexItem{
			{Version: "1.22.0", Stable: true, Files: []IndexFile{
				{OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "go1.22.0.linux-amd64.tar.gz"},
				{OS: "darwin", Arch: "amd64", Ext: "tar.gz", Filename: "go1.22.0.darwin-amd64.tar.gz"},
				{OS: "windows", Arch: "amd64", Ext: "zip", Filename: "go1.22.0.windows-amd64.zip"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("save index: %v", err)
	}

	limited, err := svc.SearchIndex("go", SearchOptions{Keywords: []string{"amd64"}, Number: 2})
	if err != nil {
		t.Fatalf("search limited index: %v", err)
	}
	assert.Eq(t, 2, len(limited))

	all, err := svc.SearchIndex("go", SearchOptions{Keywords: []string{"amd64"}, Number: 0})
	if err != nil {
		t.Fatalf("search unlimited index: %v", err)
	}
	assert.Eq(t, 3, len(all))
}

func TestServiceSearchIndexSupportsRegexAndVersionSort(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	cfg.SDK["node"] = cfgpkg.SDKSection{
		Target:          stringPtr("nodejs/node{version}"),
		URLTemplate:     stringPtr("https://example.com/node-v{version}-{os}-{arch}.{ext}"),
		IndexURL:        stringPtr("https://example.com/node/"),
		IndexFormat:     stringPtr("html"),
		FilenamePattern: stringPtr("node-v{version}-{os}-{arch}.{ext}"),
		ExtMap:          map[string]string{"linux": "tar.gz"},
		ArchMap:         map[string]string{"amd64": "x64"},
	}
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}
	err := svc.IndexCache.Save(Index{
		Schema: 1,
		SDK:    "node",
		Items: []IndexItem{
			{Version: "20.11.1", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "node-v20.11.1-linux-x64.tar.gz"}}},
			{Version: "22.0.0", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "node-v22.0.0-linux-x64.tar.gz"}}},
			{Version: "22.3.1", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "node-v22.3.1-linux-x64.tar.gz"}}},
		},
	})
	if err != nil {
		t.Fatalf("save index: %v", err)
	}

	desc, err := svc.SearchIndex("node", SearchOptions{Keywords: []string{"REG:^22"}, Sort: "desc", Number: 1})
	if err != nil {
		t.Fatalf("search desc index: %v", err)
	}
	assert.Eq(t, 1, len(desc))
	assert.Eq(t, "22.3.1", desc[0].Version)

	asc, err := svc.SearchIndex("node", SearchOptions{Keywords: []string{"REG:^22"}, Sort: "asc", Number: 0})
	if err != nil {
		t.Fatalf("search asc index: %v", err)
	}
	assert.Eq(t, 2, len(asc))
	assert.Eq(t, "22.0.0", asc[0].Version)
	assert.Eq(t, "22.3.1", asc[1].Version)

	_, err = svc.SearchIndex("node", SearchOptions{Keywords: []string{"REG:["}})
	assert.Err(t, err)
}

func TestServiceRefreshIndexReportsFetchAndParseStages(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<a href="go1.21.1.linux-amd64.zip">go</a>`)
	}))
	defer server.Close()
	cfg := testSDKConfig(root)
	goSDK := cfg.SDK["go"]
	goSDK.IndexURL = stringPtr(server.URL + "/golang/")
	cfg.SDK["go"] = goSDK

	var events []IndexRefreshEvent
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		OnIndexRefresh: func(event IndexRefreshEvent) {
			events = append(events, event)
		},
	}

	index, err := svc.RefreshIndex(context.Background(), "go")
	if err != nil {
		t.Fatalf("refresh index: %v", err)
	}
	assert.Eq(t, "go", index.SDK)
	assert.Eq(t, 4, len(events))
	assert.Eq(t, IndexRefreshFetchStart, events[0].Stage)
	assert.Eq(t, server.URL+"/golang/", events[0].URL)
	assert.Eq(t, IndexRefreshFetchDone, events[1].Stage)
	assert.Eq(t, IndexRefreshParseStart, events[2].Stage)
	assert.Eq(t, "html", events[2].Format)
	assert.Eq(t, IndexRefreshParseDone, events[3].Stage)
	assert.Eq(t, 1, events[3].Versions)
	assert.Eq(t, 1, events[3].Files)
}

func TestServiceRefreshAllIndexes(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/golang/":
			_, _ = io.WriteString(w, `<a href="go1.21.1.linux-amd64.zip">go</a>`)
		case "/node/":
			_, _ = io.WriteString(w, `<a href="node-v20.11.1-linux-x64.tar.xz">node</a>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := testSDKConfig(root)
	goSDK := cfg.SDK["go"]
	goSDK.IndexURL = stringPtr(server.URL + "/golang/")
	cfg.SDK["go"] = goSDK
	cfg.SDK["node"] = cfgpkg.SDKSection{
		Target:          stringPtr("nodejs/node{version}"),
		URLTemplate:     stringPtr("https://example.com/node-v{version}-{os}-{arch}.{ext}"),
		IndexURL:        stringPtr(server.URL + "/node/"),
		IndexFormat:     stringPtr("html"),
		FilenamePattern: stringPtr("node-v{version}-{os}-{arch}.{ext}"),
		ExtMap:          map[string]string{"linux": "tar.xz"},
		ArchMap:         map[string]string{"amd64": "x64"},
	}
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}

	indexes, err := svc.RefreshAllIndexes(context.Background())
	if err != nil {
		t.Fatalf("refresh all indexes: %v", err)
	}
	assert.Eq(t, 2, len(indexes))

	if err := svc.ClearAllIndexes(); err != nil {
		t.Fatalf("clear all indexes: %v", err)
	}
	infos, err := svc.ListIndexes()
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	assert.Eq(t, 0, len(infos))
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

func testSDKConfig(root string) *cfgpkg.File {
	cfg := cfgpkg.NewFile()
	cfg.Global.SDKTarget = stringPtr(filepath.Join(root, "sdks"))
	cfg.Global.SDKExtMap = map[string]string{"linux": "zip"}
	cfg.SDK["go"] = cfgpkg.SDKSection{
		Target:          stringPtr("gosdk/go{version}"),
		URLTemplate:     stringPtr("https://example.com/go{version}.{os}-{arch}.{ext}"),
		IndexURL:        stringPtr("https://example.com/golang/"),
		IndexFormat:     stringPtr("html"),
		FilenamePattern: stringPtr("go{version}.{os}-{arch}.{ext}"),
		StripComponents: intPtr(1),
	}
	return cfg
}

func writeSDKZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.WriteFile(path, sdkZipBytes(t, files), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}

func sdkZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func stringLen(data []byte) string {
	return intString(len(data))
}

func intString(value int) string {
	return strconv.Itoa(value)
}
