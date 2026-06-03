package cache

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestCacheServeUIRendersFileList(t *testing.T) {
	cacheDir := t.TempDir()
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip"), "pkg")
	writeCacheTestFile(t, filepath.Join(cacheDir, "sdk-downloads/go/go&1.zip"), "sdk")

	handler := NewHandler(Service{Now: fixedNow}, cacheDir, ServeOptions{
		Root:    "all",
		Version: "1.2.3",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	assert.StrContains(t, rec.Header().Get("Content-Type"), "text/html")
	body := rec.Body.String()
	assert.StrContains(t, body, "eget-cache")
	assert.StrContains(t, body, "1.2.3")
	assert.StrContains(t, body, "root: all")
	assert.StrContains(t, body, "2 files")
	assert.StrContains(t, body, "/files/pkg.zip")
	assert.StrContains(t, body, "pkg.zip")
	assert.StrContains(t, body, "sdk-downloads/go/go&amp;1.zip")
	assert.StrContains(t, body, "data-kind=\"sdk\"")
	assert.StrContains(t, body, "Search files")
	assert.StrContains(t, body, "Kind")
	assert.StrContains(t, body, `class="search-input"`)
	assert.NotContains(t, body, `<select id="kind"`)
	assert.StrContains(t, body, `type="checkbox" value="pkg"`)
	assert.StrContains(t, body, `type="checkbox" value="api"`)
	assert.StrContains(t, body, `type="checkbox" value="sdk"`)
	assert.StrContains(t, body, `type="checkbox" value="sdk-index"`)
	assert.StrContains(t, body, "selectedKinds")
}

func TestCacheServeUIHonorsRootScope(t *testing.T) {
	cacheDir := t.TempDir()
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip"), "pkg")
	writeCacheTestFile(t, filepath.Join(cacheDir, "sdk-downloads/go/go.zip"), "sdk")

	handler := NewHandler(Service{Now: fixedNow}, cacheDir, ServeOptions{
		Root:    "sdk",
		Version: "1.2.3",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.StrContains(t, body, "1 file")
	assert.StrContains(t, body, "sdk-downloads/go/go.zip")
	assert.NotContains(t, body, "pkg.zip")
}

func TestCacheServeUIAllowsNoIndex(t *testing.T) {
	cacheDir := t.TempDir()
	writeCacheTestFile(t, filepath.Join(cacheDir, "pkg.zip"), strings.Repeat("a", 8))

	handler := NewHandler(Service{Now: fixedNow}, cacheDir, ServeOptions{
		Root:    "all",
		NoIndex: true,
		Version: "1.2.3",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	assert.StrContains(t, rec.Body.String(), "pkg.zip")
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 27, 10, 30, 0, 0, time.UTC)
}
