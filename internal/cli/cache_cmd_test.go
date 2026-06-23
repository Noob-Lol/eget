package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/gookit/goutil/x/ccolor"
	appcache "github.com/inherelab/eget/internal/app/cache"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

func TestCliServiceHandleCacheCleanDryRun(t *testing.T) {
	tmp := newCLICacheDir(t)
	writeCLITestFile(t, filepath.Join(tmp, "old.zip"), "old")
	old := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	assert.NoErr(t, os.Chtimes(filepath.Join(tmp, "old.zip"), old, old))

	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	var stderr bytes.Buffer
	service := &cliService{
		cacheService: appcache.Service{
			Config: cfg,
			Now: func() time.Time {
				return time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
			},
		},
		stderr: &stderr,
	}

	err := service.handleCacheClean(&CacheCleanOptions{Older: "3d", DryRun: true})

	assert.NoErr(t, err)
	out := ccolor.ClearCode(stderr.String())
	assert.Contains(t, out, "Dry run: eget cache clean")
	assert.Contains(t, out, "matched files: 1")
	assert.True(t, fileExistsCLI(filepath.Join(tmp, "old.zip")))
}

func TestCliServiceHandleCacheCleanLargeDeletionRequiresYesInNonTTY(t *testing.T) {
	tmp := newCLICacheDir(t)
	for i := 0; i < 100; i++ {
		writeCLITestFile(t, filepath.Join(tmp, fmt.Sprintf("pkg-%03d.zip", i)), "pkg")
	}
	reader, writer, err := os.Pipe()
	assert.NoErr(t, err)
	assert.NoErr(t, writer.Close())
	origStdin := os.Stdin
	os.Stdin = reader
	defer func() {
		os.Stdin = origStdin
		assert.NoErr(t, reader.Close())
	}()

	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := &cliService{
		cacheService: appcache.Service{Config: cfg},
		stderr:       io.Discard,
	}

	err = service.handleCacheClean(&CacheCleanOptions{Older: "3d", All: true})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "--yes")
	assert.True(t, fileExistsCLI(filepath.Join(tmp, "pkg-000.zip")))
}

func TestCliServiceHandleCacheCleanLargeDeletionYesSkipsConfirmation(t *testing.T) {
	tmp := newCLICacheDir(t)
	for i := 0; i < 100; i++ {
		writeCLITestFile(t, filepath.Join(tmp, fmt.Sprintf("pkg-%03d.zip", i)), "pkg")
	}

	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	var stderr bytes.Buffer
	service := &cliService{
		cacheService: appcache.Service{Config: cfg},
		stderr:       &stderr,
	}

	err := service.handleCacheClean(&CacheCleanOptions{Older: "3d", All: true, Yes: true})

	assert.NoErr(t, err)
	assert.Contains(t, ccolor.ClearCode(stderr.String()), "removed files: 100")
	assert.False(t, fileExistsCLI(filepath.Join(tmp, "pkg-000.zip")))
}

func TestCliServiceHandleCacheListJSON(t *testing.T) {
	tmp := newCLICacheDir(t)
	writeCLITestFile(t, filepath.Join(tmp, "pkg-cache", "tool.zip"), "pkg")
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := &cliService{cacheService: appcache.Service{Config: cfg}}

	err := service.handleCacheList(&CacheListOptions{JSON: true})

	assert.NoErr(t, err)
}

func TestCliServiceHandleCacheStatusText(t *testing.T) {
	tmp := newCLICacheDir(t)
	writeCLITestFile(t, filepath.Join(tmp, "pkg-cache", "tool.zip"), "pkg")
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	var stderr bytes.Buffer
	service := &cliService{cacheService: appcache.Service{Config: cfg}, stderr: &stderr}

	err := service.handleCacheStatus(&CacheStatusOptions{})

	assert.NoErr(t, err)
	assert.Contains(t, stderr.String(), "Cache status")
	assert.Contains(t, stderr.String(), "cache dir:")
}

func TestCliServiceHandleCacheCleanDryRunJSON(t *testing.T) {
	tmp := newCLICacheDir(t)
	writeCLITestFile(t, filepath.Join(tmp, "old.zip"), "old")
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := &cliService{cacheService: appcache.Service{Config: cfg}}

	err := service.handleCacheClean(&CacheCleanOptions{Older: "3d", DryRun: true, JSON: true})

	assert.NoErr(t, err)
	assert.True(t, fileExistsCLI(filepath.Join(tmp, "old.zip")))
}

func writeCLITestFile(t *testing.T, path, body string) {
	t.Helper()
	assert.NoErr(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NoErr(t, os.WriteFile(path, []byte(body), 0o644))
}

func newCLICacheDir(t *testing.T) string {
	t.Helper()
	cacheDir := filepath.Join(t.TempDir(), "eget")
	assert.NoErr(t, os.MkdirAll(cacheDir, 0o755))
	return cacheDir
}

func fileExistsCLI(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
