package cache

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

func TestCacheServerTokenProtectsManifest(t *testing.T) {
	handler := NewHandler(Service{}, t.TempDir(), ServeOptions{Token: "secret"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/manifest.json", nil))
	assert.Eq(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "Bearer")

	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	req.Header.Set("Authorization", "Bearer secret")
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, req)
	assert.Eq(t, http.StatusOK, okRec.Code)
}

func TestCacheServerTokenAllowsHealthz(t *testing.T) {
	handler := NewHandler(Service{}, t.TempDir(), ServeOptions{Token: "secret"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Eq(t, http.StatusOK, rec.Code)
}

func TestCacheServerJSONLogWritesOneLineWithoutQueryOrToken(t *testing.T) {
	var log bytes.Buffer
	handler := NewHandler(Service{}, t.TempDir(), ServeOptions{
		Token:     "secret",
		JSONLog:   true,
		LogWriter: &log,
	})
	req := httptest.NewRequest(http.MethodGet, "/manifest.json?token=bad", nil)
	req.Header.Set("Authorization", "Bearer secret")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	var event map[string]any
	assert.NoErr(t, json.Unmarshal(bytes.TrimSpace(log.Bytes()), &event))
	assert.Eq(t, "/manifest.json", event["path"])
	assert.Eq(t, float64(200), event["status"])
	assert.False(t, strings.Contains(log.String(), "secret"))
	assert.False(t, strings.Contains(log.String(), "token=bad"))
}

func TestCacheServerTextLogWritesByDefaultWithoutQueryOrToken(t *testing.T) {
	var log bytes.Buffer
	handler := NewHandler(Service{
		Now: func() time.Time {
			return time.Date(2026, 6, 2, 18, 20, 30, 0, time.FixedZone("CST", 8*60*60))
		},
	}, t.TempDir(), ServeOptions{
		Token:     "secret",
		LogWriter: &log,
	})
	req := httptest.NewRequest(http.MethodGet, "/manifest.json?token=bad", nil)
	req.Header.Set("Authorization", "Bearer secret")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Eq(t, http.StatusOK, rec.Code)
	got := log.String()
	assert.Contains(t, got, "06-02 18:20:30")
	assert.False(t, strings.Contains(got, "2026-06-02T18:20:30"))
	assert.Contains(t, got, "GET /manifest.json 200")
	assert.Contains(t, got, "bytes=")
	assert.Contains(t, got, "duration_ms=")
	assert.False(t, strings.Contains(got, "secret"))
	assert.False(t, strings.Contains(got, "token=bad"))
}
