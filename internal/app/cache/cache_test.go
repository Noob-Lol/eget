package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/gookit/goutil/testutil/assert"
)

func TestParseOlderDuration(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{"minutes", "30m", 30 * time.Minute},
		{"hours", "12h", 12 * time.Hour},
		{"days", "3d", 72 * time.Hour},
		{"weeks", "1w", 7 * 24 * time.Hour},
		{"go duration", "72h", 72 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOlderDuration(tt.input)
			assert.NoErr(t, err)
			assert.Eq(t, tt.want, got)
		})
	}
}

func TestParseOlderDurationRejectsInvalidInput(t *testing.T) {
	tests := []string{"", "0", "0d", "-1d", "1mo", "abc"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseOlderDuration(input)
			assert.Err(t, err)
		})
	}
}

func TestServiceResolveCacheDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := Service{Config: cfg}

	got, err := service.ResolveCacheDir()

	assert.NoErr(t, err)
	assert.Eq(t, tmp, got)
}

func TestServiceResolveCacheDirUsesDefault(t *testing.T) {
	service := Service{Config: cfgpkg.NewFile()}

	got, err := service.ResolveCacheDir()

	assert.NoErr(t, err)
	assert.Contains(t, got, ".cache")
	assert.Contains(t, got, "eget")
}

func TestServiceRejectsDangerousCacheDir(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{"empty", ""},
		{"root", filepath.VolumeName(filepath.Clean(os.TempDir())) + string(filepath.Separator)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCacheDirForMutation(tt.dir)
			assert.Err(t, err)
		})
	}
}

func TestServiceScanClassifiesEntries(t *testing.T) {
	cacheDir := t.TempDir()
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip"), "pkg")
	writeCacheTestFile(t, filepath.Join(cacheDir, "api-cache", "repo.json"), "{}")
	writeCacheTestFile(t, filepath.Join(cacheDir, "sdk-downloads", "go", "1.22.0", "go.zip"), "sdk")
	writeCacheTestFile(t, filepath.Join(cacheDir, "sdk-index", "go.json"), "{}")
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip.part"), "partial")
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip.meta.json"), "{}")

	service := Service{}
	entries, err := service.Scan(cacheDir, CacheScanOptions{Kinds: []Kind{
		KindPkg,
		KindAPI,
		KindSDK,
		KindSDKIndex,
		KindPartial,
	}})

	assert.NoErr(t, err)
	got := map[string]Kind{}
	for _, entry := range entries {
		got[entry.RelPath] = entry.Kind
	}
	assert.Eq(t, KindPkg, got["pkg.zip"])
	assert.Eq(t, KindAPI, got["api-cache/repo.json"])
	assert.Eq(t, KindSDK, got["sdk-downloads/go/1.22.0/go.zip"])
	assert.Eq(t, KindSDKIndex, got["sdk-index/go.json"])
	assert.Eq(t, KindPartial, got["pkg.zip.part"])
	assert.Eq(t, KindPartial, got["pkg.zip.meta.json"])
}

func TestServiceDefaultCleanKindsExcludeSDKIndex(t *testing.T) {
	kinds := normalizeKinds(nil)

	assert.Eq(t, []Kind{KindPkg, KindAPI, KindSDK, KindPartial}, kinds)
}

func writeCacheTestFile(t *testing.T, path, body string) {
	t.Helper()
	assert.NoErr(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NoErr(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestServicePreviewCleanDoesNotRemoveFiles(t *testing.T) {
	cacheDir := t.TempDir()
	oldFile := filepath.Join(cacheDir, "old.zip")
	writeCacheTestFile(t, oldFile, "old")
	oldTime := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	assert.NoErr(t, os.Chtimes(oldFile, oldTime, oldTime))

	service := Service{Now: func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}}
	result, err := service.PreviewClean(cacheDir, CleanOptions{Older: 3 * 24 * time.Hour})

	assert.NoErr(t, err)
	assert.Eq(t, 1, result.MatchedFiles)
	assert.Eq(t, 0, result.RemovedFiles)
	assert.True(t, fileExistsForTest(oldFile))
}

func TestServiceCleanRemovesOnlyOlderMatchedFiles(t *testing.T) {
	cacheDir := t.TempDir()
	oldFile := filepath.Join(cacheDir, "old.zip")
	newFile := filepath.Join(cacheDir, "new.zip")
	writeCacheTestFile(t, oldFile, "old")
	writeCacheTestFile(t, newFile, "new")
	oldTime := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	assert.NoErr(t, os.Chtimes(oldFile, oldTime, oldTime))
	assert.NoErr(t, os.Chtimes(newFile, newTime, newTime))

	service := Service{Now: func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}}
	result, err := service.Clean(cacheDir, CleanOptions{Older: 3 * 24 * time.Hour})

	assert.NoErr(t, err)
	assert.Eq(t, 1, result.MatchedFiles)
	assert.Eq(t, 1, result.RemovedFiles)
	assert.False(t, fileExistsForTest(oldFile))
	assert.True(t, fileExistsForTest(newFile))
}

func TestServiceCleanAllIgnoresOlder(t *testing.T) {
	cacheDir := t.TempDir()
	file := filepath.Join(cacheDir, "new.zip")
	writeCacheTestFile(t, file, "new")

	service := Service{Now: func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}}
	result, err := service.Clean(cacheDir, CleanOptions{Older: 3 * 24 * time.Hour, All: true})

	assert.NoErr(t, err)
	assert.Eq(t, 1, result.RemovedFiles)
	assert.False(t, fileExistsForTest(file))
}

func TestServiceCleanDoesNotRemoveSDKIndexByDefault(t *testing.T) {
	cacheDir := t.TempDir()
	indexFile := filepath.Join(cacheDir, "sdk-index", "go.json")
	writeCacheTestFile(t, indexFile, "{}")
	oldTime := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	assert.NoErr(t, os.Chtimes(indexFile, oldTime, oldTime))

	service := Service{Now: func() time.Time {
		return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	}}
	result, err := service.Clean(cacheDir, CleanOptions{Older: 3 * 24 * time.Hour})

	assert.NoErr(t, err)
	assert.Eq(t, 0, result.RemovedFiles)
	assert.True(t, fileExistsForTest(indexFile))
}

func TestServiceCleanRemovesSDKIndexWhenExplicit(t *testing.T) {
	cacheDir := t.TempDir()
	indexFile := filepath.Join(cacheDir, "sdk-index", "go.json")
	writeCacheTestFile(t, indexFile, "{}")

	service := Service{}
	result, err := service.Clean(cacheDir, CleanOptions{All: true, Kinds: []Kind{KindSDKIndex}})

	assert.NoErr(t, err)
	assert.Eq(t, 1, result.RemovedFiles)
	assert.False(t, fileExistsForTest(indexFile))
}

func TestServicePreviewCleanReportsLargeDeletionNeed(t *testing.T) {
	cacheDir := t.TempDir()
	for i := 0; i < 100; i++ {
		writeCacheTestFile(t, filepath.Join(cacheDir, fmt.Sprintf("pkg-%03d.zip", i)), "pkg")
	}

	service := Service{}
	result, err := service.PreviewClean(cacheDir, CleanOptions{All: true})

	assert.NoErr(t, err)
	assert.Eq(t, 100, result.MatchedFiles)
	assert.True(t, result.NeedsConfirmation())
	assert.True(t, fileExistsForTest(filepath.Join(cacheDir, "pkg-000.zip")))
}

func fileExistsForTest(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
