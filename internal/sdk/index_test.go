package sdk

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestIndexCacheSaveLoadListAndClear(t *testing.T) {
	cache := IndexCache{Dir: t.TempDir()}
	fetchedAt := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	index := Index{
		Schema:    1,
		SDK:       "go",
		SourceURL: "https://example.com/golang/",
		FetchedAt: fetchedAt,
		Items: []IndexItem{
			{Version: "1.21.1", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "tar.gz", URL: "https://example.com/go.tar.gz", Filename: "go.tar.gz"}}},
		},
	}

	if err := cache.Save(index); err != nil {
		t.Fatalf("save index: %v", err)
	}
	assert.Eq(t, filepath.Join(cache.Dir, "go.json"), cache.Path("go"))

	loaded, err := cache.Load("go")
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	assert.Eq(t, "go", loaded.SDK)
	assert.Eq(t, fetchedAt, loaded.FetchedAt)

	items, err := cache.List()
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	assert.Eq(t, 1, len(items))
	assert.Eq(t, "go", items[0].SDK)
	assert.Eq(t, 1, items[0].Versions)
	assert.Eq(t, "https://example.com/golang/", items[0].SourceURL)

	if err := cache.Clear("go"); err != nil {
		t.Fatalf("clear index: %v", err)
	}
	_, err = cache.Load("go")
	assert.Err(t, err)
}

func TestSelectVersion(t *testing.T) {
	index := Index{Items: []IndexItem{
		{Version: "1.21.1", Stable: true},
		{Version: "1.21.13", Stable: true},
		{Version: "1.22.0-rc1", Stable: false},
		{Version: "1.22.2", Stable: true},
	}}

	item, err := SelectVersion(index, Target{Name: "go", Version: "latest", Kind: VersionLatest})
	if err != nil {
		t.Fatalf("select latest: %v", err)
	}
	assert.Eq(t, "1.22.2", item.Version)

	item, err = SelectVersion(index, Target{Name: "go", Version: "1.21", Kind: VersionPrefix})
	if err != nil {
		t.Fatalf("select prefix: %v", err)
	}
	assert.Eq(t, "1.21.13", item.Version)

	item, err = SelectVersion(index, Target{Name: "go", Version: "1.21.1", Kind: VersionExact})
	if err != nil {
		t.Fatalf("select exact: %v", err)
	}
	assert.Eq(t, "1.21.1", item.Version)

	_, err = SelectVersion(index, Target{Name: "go", Version: "1.20", Kind: VersionPrefix})
	assert.Err(t, err)
}

func TestSelectFile(t *testing.T) {
	item := IndexItem{Files: []IndexFile{
		{OS: "windows", Arch: "amd64", Ext: "zip"},
		{OS: "linux", Arch: "amd64", Ext: "tar.gz"},
	}}

	file, err := SelectFile(item, "linux", "amd64", "tar.gz")
	if err != nil {
		t.Fatalf("select file: %v", err)
	}
	assert.Eq(t, "linux", file.OS)

	_, err = SelectFile(item, "darwin", "amd64", "tar.gz")
	assert.Err(t, err)
}
