package client

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/config"
)

func SetVerbose(enabled bool, writer io.Writer) {
	verboseEnabled = enabled
	if writer == nil {
		verboseWriter = io.Discard
		return
	}
	verboseWriter = writer
}

func SetProxyNoticeWriter(writer io.Writer) io.Writer {
	prev := proxyNoticeWriter
	proxyNoticeWriter = writer
	return prev
}

func SetAPICacheNoticeWriter(writer io.Writer) io.Writer {
	prev := apiCacheNoticeWriter
	apiCacheNoticeWriter = writer
	return prev
}

func SetHTTPDoForTest(fn func(client *http.Client, req *http.Request) (*http.Response, error)) func() {
	prev := httpDo
	httpDo = fn
	return func() { httpDo = prev }
}

func SetDownloadGetWithOptionsForTest(fn func(url string, opts Options) (*http.Response, error)) func() {
	prev := downloadGetWithOptions
	downloadGetWithOptions = fn
	return func() { downloadGetWithOptions = prev }
}

func printProxyNotice(kind, proxyURL string) {
	if proxyURL == "" || proxyNoticeWriter == nil {
		return
	}
	key := proxyNoticeKey(kind, proxyURL, proxyNoticeWriter)
	proxyNoticeMu.Lock()
	if _, ok := proxyNoticeSeen[key]; ok {
		proxyNoticeMu.Unlock()
		return
	}
	proxyNoticeSeen[key] = struct{}{}
	proxyNoticeMu.Unlock()
	ccolor.Fprintf(proxyNoticeWriter, " - Using <ylw>http_proxy for %s</>: %s\n", kind, proxyURL)
}

func shouldUseConfiguredProxyURL(parsed *url.URL, proxyURL string, exclude []string) bool {
	if strings.TrimSpace(proxyURL) == "" {
		return false
	}
	if parsed == nil {
		return false
	}
	return !config.ProxyExcluded(parsed.Host, exclude)
}

func shouldUseConfiguredProxy(rawURL, proxyURL string, exclude []string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return shouldUseConfiguredProxyURL(parsed, proxyURL, exclude)
}

func downloadNoticeURL(rawURL string, opts Options) string {
	parsed, err := urlpkgParse(rawURL)
	if err != nil {
		return rawURL
	}
	attempts := requestAttemptURLs(rawURL, parsed, opts)
	if len(attempts) == 0 {
		return rawURL
	}
	return attempts[0]
}

func proxyNoticeKey(kind, proxyURL string, writer io.Writer) string {
	return kind + "\x00" + proxyURL + "\x00" + writerIdentity(writer)
}

func writerIdentity(writer io.Writer) string {
	if writer == nil {
		return ""
	}
	value := reflect.ValueOf(writer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return fmt.Sprintf("%T:%x", writer, value.Pointer())
	default:
		return fmt.Sprintf("%T", writer)
	}
}

func resetProxyNoticeStateForTest() {
	proxyNoticeMu.Lock()
	defer proxyNoticeMu.Unlock()
	proxyNoticeSeen = map[string]struct{}{}
}

func printAPICacheNotice(cachePath string) {
	if cachePath == "" || apiCacheNoticeWriter == nil {
		return
	}
	ccolor.Fprintf(apiCacheNoticeWriter, " - Using <ylw>api_cache file</>: %s\n", filepath.Base(cachePath))
}

func verbosef(format string, args ...any) {
	Verbosef(format, args...)
}

func Verbosef(format string, args ...any) {
	if !verboseEnabled || verboseWriter == nil {
		return
	}
	ccolor.Fprintf(verboseWriter, "<ylw>verbose</> "+format+"\n", args...)
}

func VerboseEnabledForTest() bool {
	return verboseEnabled
}
