package urltemplate

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestFinderFindsRenderedURLFromLatest(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://example.com/latest": "1.2.3",
	}}
	finder := Finder{
		Name:   "claude",
		Target: Target{ID: "claude", Normalized: "template:claude"},
		Config: Config{
			LatestURL:   "https://example.com/latest",
			URLTemplate: "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:       map[string]string{"windows": "win32"},
			ArchMap:     map[string]string{"amd64": "x64"},
			ExtMap:      map[string]string{"windows": ".exe"},
		},
		GOOS:   "windows",
		GOARCH: "amd64",
		Getter: getter,
	}

	assets, err := finder.Find()
	assert.NoErr(t, err)
	assert.Eq(t, []string{"https://example.com/1.2.3/win32-x64/claude.exe"}, assets)
	assert.Eq(t, []string{"https://example.com/latest"}, getter.requests)
	assert.Eq(t, "1.2.3", finder.Version)
}

func TestFinderLatestReadsYAMLPublishedAt(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://example.com/latest.yaml": "version: v1.2.5\nreleased_at: 2026-05-25T10:20:30Z\n",
	}}
	finder := Finder{
		Name:   "markview",
		Target: Target{ID: "markview", Normalized: "template:markview"},
		Config: Config{
			LatestURL:    "https://example.com/latest.yaml",
			LatestFormat: "yaml",
		},
		Getter: getter,
	}

	info, err := finder.Latest()

	assert.NoErr(t, err)
	assert.Eq(t, "v1.2.5", info.Version)
	assert.Eq(t, time.Date(2026, 5, 25, 10, 20, 30, 0, time.UTC), info.PublishedAt)
}

type fakeGetter struct {
	responses map[string]string
	requests  []string
}

func (f *fakeGetter) Get(url string) (*http.Response, error) {
	f.requests = append(f.requests, url)
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(f.responses[url])),
	}, nil
}
