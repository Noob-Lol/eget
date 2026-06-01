package client

import (
	"bytes"
	"io"
	"net/http"
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
	assert.Eq(t, 1, strings.Count(got, "proxy_url for GitHub API request"))
	assert.Eq(t, 1, strings.Count(got, "proxy_url for download request"))
}
