package client

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	"github.com/gookit/goutil/x/ccolor"
)

func TestProxyNoticePrintsOncePerKindAndProxyURL(t *testing.T) {
	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	origHTTPDo := httpDo
	origDownloadGet := downloadGetWithOptions
	defer func() {
		proxyNoticeWriter = origNoticeWriter
		httpDo = origHTTPDo
		downloadGetWithOptions = origDownloadGet
		resetProxyNoticeStateForTest()
	}()
	proxyNoticeWriter = &notice
	resetProxyNoticeStateForTest()
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`download`)),
		}, nil
	}

	opts := Options{ProxyURL: "http://127.0.0.1:7890", ChunkConcurrency: 1}
	for i := 0; i < 2; i++ {
		resp, err := GetWithOptions("https://api.github.com/repos/gookit/gitw/releases/latest", opts)
		assert.NoErr(t, err)
		_ = resp.Body.Close()
		assert.NoErr(t, Download("https://example.com/tool.tar.gz", io.Discard, func(int64) io.Writer {
			return io.Discard
		}, opts))
	}

	got := ccolor.ClearCode(notice.String())
	assert.Eq(t, 1, strings.Count(got, "http_proxy for GitHub API request"))
	assert.Eq(t, 1, strings.Count(got, "http_proxy for download request"))
}

func TestProxyFuncForSkipsExcludedHost(t *testing.T) {
	proxyFunc, err := ProxyFuncFor("http://127.0.0.1:7890", []string{"github.com"})
	assert.NoErr(t, err)

	cases := []struct {
		name string
		url  string
		want *url.URL
	}{
		{name: "exact host", url: "https://github.com/owner/repo", want: nil},
		{name: "subdomain", url: "https://api.github.com/repos/owner/repo", want: nil},
		{name: "non excluded", url: "https://example.com/file.zip", want: mustParseURLForTest("http://127.0.0.1:7890")},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := proxyFunc(httptest.NewRequest(http.MethodGet, tt.url, nil))
			assert.NoErr(t, err)
			assert.Eq(t, tt.want, got)
		})
	}
}

func TestGetWithOptionsSkipsProxyNoticeForExcludedGitHubAPI(t *testing.T) {
	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	origHTTPDo := httpDo
	defer func() {
		proxyNoticeWriter = origNoticeWriter
		httpDo = origHTTPDo
		resetProxyNoticeStateForTest()
	}()
	proxyNoticeWriter = &notice
	resetProxyNoticeStateForTest()
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}

	cases := []struct {
		name    string
		rawURL  string
		exclude []string
		want    string
	}{
		{name: "excluded", rawURL: "https://api.github.com/repos/gookit/gitw/releases/latest", exclude: []string{"github.com"}, want: ""},
		{name: "not excluded", rawURL: "https://api.github.com/repos/gookit/gitw/releases/latest", exclude: []string{"example.com"}, want: "http_proxy for GitHub API request"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			notice.Reset()
			resetProxyNoticeStateForTest()
			resp, err := GetWithOptions(tt.rawURL, Options{ProxyURL: "http://127.0.0.1:7890", ProxyExclude: tt.exclude})
			assert.NoErr(t, err)
			_ = resp.Body.Close()

			got := ccolor.ClearCode(notice.String())
			if tt.want == "" {
				assert.Eq(t, "", got)
				return
			}
			assert.Contains(t, got, tt.want)
		})
	}
}

func mustParseURLForTest(rawURL string) *url.URL {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return parsed
}
