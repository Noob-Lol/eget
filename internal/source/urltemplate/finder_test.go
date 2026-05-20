package urltemplate

import (
	"io"
	"net/http"
	"strings"
	"testing"

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
