package sdk

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

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

func TestServiceListIndexesUsesConfiguredSDKs(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	nodeSDK := cfg.SDK["go"]
	nodeSDK.IndexURL = stringPtr("https://nodejs.org/dist/")
	cfg.SDK["node"] = nodeSDK
	jdkSDK := cfg.SDK["go"]
	jdkSDK.IndexURL = stringPtr("https://mirrors.huaweicloud.com/openjdk/")
	cfg.SDK["jdk"] = jdkSDK
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}
	err := svc.IndexCache.Save(Index{
		Schema:    1,
		SDK:       "jdk",
		SourceURL: "https://mirrors.huaweicloud.com/openjdk/",
		FetchedAt: time.Date(2026, 5, 22, 18, 48, 39, 0, time.UTC),
		Items: []IndexItem{
			{Version: "21.0.2", Stable: true},
			{Version: "22.0.2", Stable: true},
		},
	})
	if err != nil {
		t.Fatalf("save configured index: %v", err)
	}
	err = svc.IndexCache.Save(Index{
		Schema:    1,
		SDK:       "jdk",
		SourceURL: "https://old.example.com/openjdk/",
		FetchedAt: time.Date(2026, 5, 22, 11, 55, 49, 0, time.UTC),
		Items:     []IndexItem{{Version: "17.0.1", Stable: true}},
	})
	if err != nil {
		t.Fatalf("save stale index: %v", err)
	}

	infos, err := svc.ListIndexes()
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}

	assert.Eq(t, 3, len(infos))
	assert.Eq(t, "go", infos[0].SDK)
	assert.False(t, infos[0].Cached)
	assert.Eq(t, 0, infos[0].Versions)
	assert.True(t, infos[0].FetchedAt.IsZero())
	assert.Eq(t, "https://example.com/golang/", infos[0].SourceURL)
	assert.Eq(t, "jdk", infos[1].SDK)
	assert.True(t, infos[1].Cached)
	assert.Eq(t, 2, infos[1].Versions)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", infos[1].SourceURL)
	assert.Eq(t, "node", infos[2].SDK)
	assert.False(t, infos[2].Cached)
	assert.Eq(t, "https://nodejs.org/dist/", infos[2].SourceURL)
}

func TestServiceRefreshIndexUsesBrowserHeaders(t *testing.T) {
	root := t.TempDir()
	var userAgent string
	var accept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		accept = r.Header.Get("Accept")
		_, _ = io.WriteString(w, `<a href="go1.21.1.linux-amd64.zip">go</a>`)
	}))
	defer server.Close()
	cfg := testSDKConfig(root)
	goSDK := cfg.SDK["go"]
	goSDK.IndexURL = stringPtr(server.URL + "/golang/")
	cfg.SDK["go"] = goSDK
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}

	_, err := svc.RefreshIndex(context.Background(), "go")
	if err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	assert.Eq(t, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36", userAgent)
	assert.Contains(t, accept, "text/html")
}

func TestServiceRefreshIndexUsesConfiguredUserAgent(t *testing.T) {
	root := t.TempDir()
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		_, _ = io.WriteString(w, `<a href="go1.21.1.linux-amd64.zip">go</a>`)
	}))
	defer server.Close()
	cfg := testSDKConfig(root)
	cfg.Global.UserAgent = stringPtr("custom-agent/1.0")
	goSDK := cfg.SDK["go"]
	goSDK.IndexURL = stringPtr(server.URL + "/golang/")
	cfg.SDK["go"] = goSDK
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}

	_, err := svc.RefreshIndex(context.Background(), "go")
	if err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	assert.Eq(t, "custom-agent/1.0", userAgent)
}

func TestServiceRefreshJSONIndexFollowsPagination(t *testing.T) {
	root := t.TempDir()
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("page"))
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("X-Pagination", `{"page":1,"next_page":2}`)
			_, _ = io.WriteString(w, `[{"version":"go1.21.1","stable":true,"files":[{"filename":"go1.21.1.linux-amd64.tar.gz","os":"linux","arch":"amd64","kind":"archive"}]}]`)
		case "2":
			w.Header().Set("X-Pagination", `{"page":2}`)
			_, _ = io.WriteString(w, `[{"version":"go1.22.0","stable":true,"files":[{"filename":"go1.22.0.linux-amd64.tar.gz","os":"linux","arch":"amd64","kind":"archive"}]}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := testSDKConfig(root)
	goSDK := cfg.SDK["go"]
	goSDK.IndexURL = stringPtr(server.URL + "/golang/?mode=json")
	goSDK.IndexFormat = stringPtr("json")
	goSDK.IndexParser = stringPtr("go-json")
	cfg.SDK["go"] = goSDK
	svc := Service{
		Config:     cfg,
		IndexCache: IndexCache{Dir: filepath.Join(root, "index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}

	index, err := svc.RefreshIndex(context.Background(), "go")
	if err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	assert.Eq(t, []string{"", "2"}, pages)
	assert.Eq(t, 2, len(index.Items))
	assert.Eq(t, "1.21.1", index.Items[0].Version)
	assert.Eq(t, "1.22.0", index.Items[1].Version)
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
	assert.Eq(t, 2, len(infos))
	assert.Eq(t, "go", infos[0].SDK)
	assert.False(t, infos[0].Cached)
	assert.Eq(t, "node", infos[1].SDK)
	assert.False(t, infos[1].Cached)
}
