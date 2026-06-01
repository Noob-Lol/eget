package client

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestGetWithOptionsUsesAPICacheForKnownProviderMetadataRequests(t *testing.T) {
	for _, tt := range []struct {
		name   string
		apiURL string
		body   string
	}{
		{
			name:   "gitlab release api",
			apiURL: "https://gitlab.com/api/v4/projects/fdroid%2Ffdroidserver/releases/permalink/latest",
			body:   `{"tag_name":"v2.3.4"}`,
		},
		{
			name:   "gitea release api",
			apiURL: "https://codeberg.org/api/v1/repos/forgejo/forgejo/releases/latest",
			body:   `{"tag_name":"v9.0.0"}`,
		},
		{
			name:   "sourceforge files listing",
			apiURL: "https://sourceforge.net/projects/winmerge/files/stable/",
			body:   `<html>cached sourceforge listing</html>`,
		},
		{
			name:   "sourceforge root files listing",
			apiURL: "https://sourceforge.net/projects/winmerge/files/",
			body:   `<html>cached sourceforge root listing</html>`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			cachePath := APICacheFilePath(cacheDir, tt.apiURL)
			if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
				t.Fatalf("mkdir cache dir: %v", err)
			}
			if err := os.WriteFile(cachePath, []byte(tt.body), 0o644); err != nil {
				t.Fatalf("write cache file: %v", err)
			}

			calls := 0
			restoreHTTPDo := SetHTTPDoForTest(func(client *http.Client, req *http.Request) (*http.Response, error) {
				calls++
				return jsonResponse(http.StatusOK, "200 OK", `network`), nil
			})
			defer restoreHTTPDo()

			resp, err := GetWithOptions(tt.apiURL, Options{
				APICacheEnabled: true,
				APICacheDir:     cacheDir,
				APICacheTime:    300,
			})
			if err != nil {
				t.Fatalf("GetWithOptions(): %v", err)
			}
			defer resp.Body.Close()

			got, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			assert.Eq(t, tt.body, string(got))
			assert.Eq(t, 0, calls)
		})
	}
}

func TestGetWithOptionsDoesNotUseAPICacheForDownloads(t *testing.T) {
	cacheDir := t.TempDir()
	downloadURL := "https://downloads.sourceforge.net/project/winmerge/stable/WinMerge.zip"
	cachePath := APICacheFilePath(cacheDir, downloadURL)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("cached download"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	calls := 0
	restoreHTTPDo := SetHTTPDoForTest(func(client *http.Client, req *http.Request) (*http.Response, error) {
		calls++
		return jsonResponse(http.StatusOK, "200 OK", `network download`), nil
	})
	defer restoreHTTPDo()

	resp, err := GetWithOptions(downloadURL, Options{
		APICacheEnabled: true,
		APICacheDir:     cacheDir,
		APICacheTime:    300,
	})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	assert.Eq(t, "network download", string(got))
	assert.Eq(t, 1, calls)
}
