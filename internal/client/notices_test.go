package client

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/gookit/goutil/x/ccolor"
)

func TestProxyNoticePrintsOncePerKindAndProxyURL(t *testing.T) {
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

func TestGetWithOptionsProxyNoticeUsesGhproxyAttemptHost(t *testing.T) {
	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	origHTTPDo := httpDo
	defer func() {
		proxyNoticeWriter = origNoticeWriter
		httpDo = origHTTPDo
		resetProxyNoticeStateForTest()
	}()
	proxyNoticeWriter = &notice
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}

	cases := []struct {
		name    string
		exclude []string
		want    string
	}{
		{name: "original github excluded but ghproxy allowed", exclude: []string{"github.com"}, want: "http_proxy for GitHub API request"},
		{name: "ghproxy excluded but original github allowed", exclude: []string{"gh.felicity.ac.cn"}, want: ""},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			notice.Reset()
			resetProxyNoticeStateForTest()
			resp, err := GetWithOptions("https://api.github.com/repos/gookit/gitw/releases/latest", Options{
				ProxyURL:          "http://127.0.0.1:7890",
				ProxyExclude:      tt.exclude,
				GhproxyEnabled:    true,
				GhproxyHostURL:    "https://gh.felicity.ac.cn",
				GhproxySupportAPI: true,
			})
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

func TestDownloadProxyNoticeUsesGhproxyAttemptHost(t *testing.T) {
	var notice bytes.Buffer
	origNoticeWriter := proxyNoticeWriter
	origHTTPDo := httpDo
	defer func() {
		proxyNoticeWriter = origNoticeWriter
		httpDo = origHTTPDo
		resetProxyNoticeStateForTest()
	}()
	proxyNoticeWriter = &notice
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`download`)),
		}, nil
	}

	cases := []struct {
		name    string
		exclude []string
		want    string
	}{
		{name: "original github excluded but ghproxy allowed", exclude: []string{"github.com"}, want: "http_proxy for download request"},
		{name: "ghproxy excluded but original github allowed", exclude: []string{"gh.felicity.ac.cn"}, want: ""},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			notice.Reset()
			resetProxyNoticeStateForTest()
			var out bytes.Buffer
			_, err := DownloadWithResult("https://github.com/gookit/gitw/releases/download/v0.3.6/tool.tar.gz", &out, func(int64) io.Writer {
				return io.Discard
			}, Options{
				ProxyURL:         "http://127.0.0.1:7890",
				ProxyExclude:     tt.exclude,
				GhproxyEnabled:   true,
				GhproxyHostURL:   "https://gh.felicity.ac.cn",
				ChunkConcurrency: 1,
			})
			assert.NoErr(t, err)

			got := ccolor.ClearCode(notice.String())
			if tt.want == "" {
				assert.Eq(t, "", got)
				return
			}
			assert.Contains(t, got, tt.want)
		})
	}
}

func TestDownloadProxyNoticeUsesGhproxyFallbackAttemptHost(t *testing.T) {
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
	attempts := 0
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		attempts++
		if req.URL.Host == "excluded.proxy.test" {
			return nil, errors.New("primary unavailable")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`download`)),
		}, nil
	}

	var out bytes.Buffer
	_, err := DownloadWithResult("https://github.com/gookit/gitw/releases/download/v0.3.6/tool.tar.gz", &out, func(int64) io.Writer {
		return io.Discard
	}, Options{
		ProxyURL:         "http://127.0.0.1:7890",
		ProxyExclude:     []string{"excluded.proxy.test"},
		GhproxyEnabled:   true,
		GhproxyHostURL:   "https://excluded.proxy.test",
		GhproxyFallbacks: []string{"https://allowed.proxy.test"},
		ChunkConcurrency: 1,
	})
	assert.NoErr(t, err)
	assert.Eq(t, 2, attempts)
	assert.Eq(t, "download", out.String())

	got := ccolor.ClearCode(notice.String())
	assert.Contains(t, got, "http_proxy for download request")
}

func mustParseURLForTest(rawURL string) *url.URL {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return parsed
}
