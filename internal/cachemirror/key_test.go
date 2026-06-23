package cachemirror

import (
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestKeyForRelPathNormalizesSlashPath(t *testing.T) {
	got := KeyForRelPath(`pkg-cache\tool-1.2.3-a1b2c3d4.zip`)
	assert.Eq(t, "path-md5:e3bd999bec663dd9ec8612d4f87dc7d4", got)
}

func TestRelPathForCacheFile(t *testing.T) {
	cacheDir := t.TempDir()
	fullPath := filepath.Join(cacheDir, "pkg-cache", "tool.zip")

	got, err := RelPath(cacheDir, fullPath)

	assert.NoErr(t, err)
	assert.Eq(t, "pkg-cache/tool.zip", got)
}

func TestRelPathRejectsOutsideCacheDir(t *testing.T) {
	cacheDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "tool.zip")

	_, err := RelPath(cacheDir, outside)

	assert.Err(t, err)
}

func TestDownloadURLTrimsBaseSlash(t *testing.T) {
	got, err := DownloadURL("http://mirror.local:8686/", "path-md5:abc")

	assert.NoErr(t, err)
	assert.Eq(t, "http://mirror.local:8686/download/path-md5:abc", got)
}
