package cache

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestCacheServerHealthz(t *testing.T) {
	cacheDir := t.TempDir()
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":true`)
	assert.Contains(t, rec.Body.String(), `"name":"eget-cache"`)
}

func TestCacheServerManifest(t *testing.T) {
	cacheDir := t.TempDir()
	file := filepath.Join(cacheDir, "pkg.zip")
	assert.NoErr(t, os.WriteFile(file, []byte("pkg"), 0o644))
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip.part"), []byte("partial"), 0o644))
	fixed := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	service := Service{Now: func() time.Time { return fixed }}
	handler := NewHandler(service, cacheDir, ServeOptions{})
	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	var manifest Manifest
	assert.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &manifest))
	assert.Eq(t, 1, manifest.Schema)
	assert.Eq(t, "eget-cache", manifest.Server.Name)
	assert.Eq(t, "", manifest.Cache.Root)
	assert.Eq(t, 1, len(manifest.Files))
	assert.Eq(t, "pkg", manifest.Files[0].Kind)
	assert.Eq(t, "pkg.zip", manifest.Files[0].Path)
	assert.Eq(t, "/files/pkg.zip", manifest.Files[0].URL)
	assert.Eq(t, "http://example.com", manifest.Server.BaseURL)
}

func TestCacheServerFilesDownloadHeadAndRange(t *testing.T) {
	cacheDir := t.TempDir()
	file := filepath.Join(cacheDir, "sdk-downloads", "go", "1.22.0", "go.zip")
	assert.NoErr(t, os.MkdirAll(filepath.Dir(file), 0o755))
	assert.NoErr(t, os.WriteFile(file, []byte("0123456789"), 0o644))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})

	getReq := httptest.NewRequest(http.MethodGet, "/files/sdk-downloads/go/1.22.0/go.zip", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	assert.Eq(t, http.StatusOK, getRec.Code)
	assert.Eq(t, "0123456789", getRec.Body.String())

	headReq := httptest.NewRequest(http.MethodHead, "/files/sdk-downloads/go/1.22.0/go.zip", nil)
	headRec := httptest.NewRecorder()
	handler.ServeHTTP(headRec, headReq)
	assert.Eq(t, http.StatusOK, headRec.Code)
	assert.Eq(t, "", headRec.Body.String())

	rangeReq := httptest.NewRequest(http.MethodGet, "/files/sdk-downloads/go/1.22.0/go.zip", nil)
	rangeReq.Header.Set("Range", "bytes=2-5")
	rangeRec := httptest.NewRecorder()
	handler.ServeHTTP(rangeRec, rangeReq)
	assert.Eq(t, http.StatusPartialContent, rangeRec.Code)
	assert.Eq(t, "2345", rangeRec.Body.String())
}

func TestCacheServerRejectsPathEscape(t *testing.T) {
	cacheDir := t.TempDir()
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})
	req := httptest.NewRequest(http.MethodGet, "/files/../secret.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusForbidden, rec.Code)
}

func TestCacheServerNoIndexRejectsDirectoryListing(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.MkdirAll(filepath.Join(cacheDir, "sdk-downloads"), 0o755))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{NoIndex: true})
	req := httptest.NewRequest(http.MethodGet, "/files/sdk-downloads/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusForbidden, rec.Code)
}

func TestCacheServerRootScopeFiltersManifest(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip"), []byte("pkg"), 0o644))
	assert.NoErr(t, os.MkdirAll(filepath.Join(cacheDir, "sdk-downloads"), 0o755))
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "sdk-downloads", "go.zip"), []byte("sdk"), 0o644))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{Root: "sdk"})
	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	var manifest Manifest
	assert.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &manifest))
	assert.Eq(t, 1, len(manifest.Files))
	assert.Eq(t, "sdk", manifest.Files[0].Kind)
}

func TestCacheServerRootScopeRejectsDirectFileOutsideScope(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip"), []byte("pkg"), 0o644))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{Root: "sdk"})
	req := httptest.NewRequest(http.MethodGet, "/files/pkg.zip", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusForbidden, rec.Code)
}

func TestCacheServerRejectsPartialFiles(t *testing.T) {
	cacheDir := t.TempDir()
	assert.NoErr(t, os.WriteFile(filepath.Join(cacheDir, "pkg.zip.part"), []byte("partial"), 0o644))
	handler := NewHandler(Service{}, cacheDir, ServeOptions{})
	req := httptest.NewRequest(http.MethodGet, "/files/pkg.zip.part", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusForbidden, rec.Code)
}
