package cache

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
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
