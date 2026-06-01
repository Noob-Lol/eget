package client

import (
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

var downloadGetWithOptions = GetWithOptions
var httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
	return client.Do(req)
}
var proxyNoticeWriter io.Writer = os.Stderr
var apiCacheNoticeWriter io.Writer = os.Stderr
var verboseWriter io.Writer = os.Stderr
var verboseEnabled bool
var proxyNoticeMu sync.Mutex
var proxyNoticeSeen = map[string]struct{}{}
var downloadProgressFlushInterval = 500 * time.Millisecond
var resumableDownloadMinSize int64 = 100 * 1024 * 1024

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
