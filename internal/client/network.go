package client

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gookit/cliui/progress"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/util"
	"golang.org/x/net/http/httpproxy"
)

type Options struct {
	ProxyURL          string
	APICacheEnabled   bool
	APICacheDir       string
	APICacheTime      int
	GhproxyEnabled    bool
	GhproxyHostURL    string
	GhproxySupportAPI bool
	GhproxyFallbacks  []string
	DisableSSL        bool
	ChunkConcurrency  int
}

type CacheMeta struct {
	Name    string
	Version string
}

type HTTPGetterFunc func(url string) (*http.Response, error)

func (f HTTPGetterFunc) Get(url string) (*http.Response, error) {
	return f(url)
}

var downloadGetWithOptions = GetWithOptions
var httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
	return client.Do(req)
}
var proxyNoticeWriter io.Writer = os.Stderr
var apiCacheNoticeWriter io.Writer = os.Stderr
var verboseWriter io.Writer = os.Stderr
var verboseEnabled bool

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

func tokenFrom(value string) (string, error) {
	if strings.HasPrefix(value, "@") {
		file, err := util.Expand(value[1:])
		if err != nil {
			return "", err
		}
		body, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(body), "\r\n"), nil
	}
	return value, nil
}

var ErrNoToken = errors.New("no github token")

func getGitHubToken() (string, error) {
	if os.Getenv("EGET_GITHUB_TOKEN") != "" {
		return tokenFrom(os.Getenv("EGET_GITHUB_TOKEN"))
	}
	if os.Getenv("GITHUB_TOKEN") != "" {
		return tokenFrom(os.Getenv("GITHUB_TOKEN"))
	}
	return "", ErrNoToken
}

func setAuthHeader(req *http.Request, disableSSL bool) error {
	token, err := getGitHubToken()
	if err != nil {
		if errors.Is(err, ErrNoToken) {
			return nil
		}
		fmt.Fprintln(os.Stderr, "warning: not using github token:", err)
		return nil
	}

	if req.URL.Scheme == "https" && req.Host == "api.github.com" {
		if disableSSL {
			return fmt.Errorf("cannot use GitHub token if SSL verification is disabled")
		}
		req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	}
	return nil
}

func Get(rawURL string, disableSSL bool) (*http.Response, error) {
	return GetWithOptions(rawURL, Options{DisableSSL: disableSSL})
}

func GetWithOptions(rawURL string, opts Options) (*http.Response, error) {
	client, err := newHTTPClient(opts)
	if err != nil {
		return nil, err
	}

	originalURL, err := urlpkgParse(rawURL)
	if err != nil {
		return nil, err
	}

	if isGitHubAPIRequest(originalURL) {
		printProxyNotice("GitHub API request", opts.ProxyURL)
	}

	cachePath, useAPICache := resolvedAPICachePath(opts, rawURL, originalURL)
	if useAPICache {
		if resp, ok, err := loadAPICacheResponse(cachePath, opts.APICacheTime); err != nil {
			verbosef("api cache read error: %v", err)
		} else if ok {
			verbosef("api cache hit: %s", cachePath)
			printAPICacheNotice(cachePath)
			return resp, nil
		} else {
			verbosef("api cache miss: %s", cachePath)
		}
	}

	attempts := requestAttemptURLs(rawURL, originalURL, opts)
	var lastErr error
	for i, attemptURL := range attempts {
		req, err := http.NewRequest("GET", attemptURL, nil)
		if err != nil {
			return nil, err
		}
		if err := setAuthHeader(req, opts.DisableSSL); err != nil {
			return nil, err
		}

		if attemptURL != rawURL {
			verbosef("ghproxy rewrite: %s -> %s", rawURL, attemptURL)
		}
		if len(attempts) > 1 {
			verbosef("ghproxy attempt %d/%d: %s", i+1, len(attempts), attemptURL)
		}

		verbosef("request: %s %s", req.Method, req.URL.String())
		resp, err := httpDo(client, req)
		if err != nil {
			verbosef("request error: %v", err)
			lastErr = err
			if i < len(attempts)-1 {
				verbosef("ghproxy fallback: switching to next host")
				continue
			}
			return nil, err
		}
		verbosef("response: %s %s", req.URL.String(), resp.Status)

		if useAPICache && resp.StatusCode == http.StatusOK {
			cachedResp, err := storeAPICacheResponse(cachePath, resp)
			if err != nil {
				verbosef("api cache write error: %v", err)
				return resp, nil
			}
			verbosef("api cache store: %s", cachePath)
			return cachedResp, nil
		}

		return resp, nil
	}

	return nil, lastErr
}

func NewHTTPGetter(opts Options) HTTPGetterFunc {
	return HTTPGetterFunc(func(rawURL string) (*http.Response, error) {
		return GetWithOptions(rawURL, opts)
	})
}

type RateLimitJSON struct {
	Resources map[string]RateLimit `json:"resources"`
}

type RateLimit struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

func (r RateLimit) ResetTime() time.Time {
	return time.Unix(r.Reset, 0)
}

func GetRateLimit(opts Options) (RateLimit, error) {
	resp, err := GetWithOptions("https://api.github.com/rate_limit", opts)
	if err != nil {
		return RateLimit{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RateLimit{}, err
	}

	var parsed RateLimitJSON
	if err := json.Unmarshal(body, &parsed); err != nil {
		return RateLimit{}, err
	}
	return parsed.Resources["core"], nil
}

func newHTTPClient(opts Options) (*http.Client, error) {
	proxyFunc, err := ProxyFuncFor(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: &http.Transport{
		Proxy:           proxyFunc,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.DisableSSL},
	}}, nil
}

func ProxyFuncFor(proxyURL string) (func(*http.Request) (*url.URL, error), error) {
	if proxyURL == "" {
		proxyFunc := httpproxy.FromEnvironment().ProxyFunc()
		return func(req *http.Request) (*url.URL, error) {
			return proxyFunc(req.URL)
		}, nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy_url %q: %w", proxyURL, err)
	}
	return http.ProxyURL(parsed), nil
}

type progressFinisher interface {
	Finish(...string)
}

const (
	defaultAutoChunkConcurrency = 5
	minChunkSize                = 4 * 1024 * 1024
)

type byteRange struct {
	Start int64
	End   int64
}

func Download(rawURL string, out io.Writer, getbar func(size int64) io.Writer, opts Options) error {
	if isLocalFile(rawURL) {
		file, err := os.Open(rawURL)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(out, file)
		return err
	}

	printProxyNotice("download request", opts.ProxyURL)

	if opts.ChunkConcurrency != 1 {
		if size, ok := probeRangeSupport(rawURL, opts); ok {
			chunks := effectiveChunkCount(opts.ChunkConcurrency, size)
			if chunks > 1 {
				bar := downloadProgressWriter(getbar, size)
				return downloadRangeChunks(rawURL, out, bar, size, chunks, opts)
			}
		}
	}

	resp, err := downloadGetWithOptions(rawURL, opts)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	verbosef("download response bytes: %d", resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("download error: %d: %s", resp.StatusCode, body)
	}

	bar := downloadProgressWriter(getbar, resp.ContentLength)
	_, err = io.Copy(io.MultiWriter(out, bar), resp.Body)
	if err == nil {
		if finisher, ok := bar.(progressFinisher); ok {
			finisher.Finish()
		}
	}
	return err
}

func effectiveChunkCount(requested int, size int64) int {
	if requested == 1 || size < 2*minChunkSize {
		return 1
	}
	maxBySize := int(size / minChunkSize)
	if maxBySize < 2 {
		return 1
	}
	limit := requested
	if limit <= 0 {
		limit = defaultAutoChunkConcurrency
	}
	if limit > maxBySize {
		limit = maxBySize
	}
	if limit < 2 {
		return 1
	}
	return limit
}

func probeRangeSupport(rawURL string, opts Options) (int64, bool) {
	resp, err := requestWithOptions(http.MethodHead, rawURL, "", opts)
	if err != nil {
		verbosef("range probe failed: %v", err)
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		verbosef("range probe unsupported status: %s", resp.Status)
		return 0, false
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes") {
		return 0, false
	}
	size := resp.ContentLength
	if size <= 0 {
		size, _ = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	}
	return size, size > 0
}

func splitByteRanges(size int64, chunks int) []byteRange {
	if chunks <= 1 {
		return []byteRange{{Start: 0, End: size - 1}}
	}
	ranges := make([]byteRange, 0, chunks)
	step := size / int64(chunks)
	start := int64(0)
	for i := 0; i < chunks; i++ {
		end := start + step - 1
		if i == chunks-1 {
			end = size - 1
		}
		ranges = append(ranges, byteRange{Start: start, End: end})
		start = end + 1
	}
	return ranges
}

func downloadRangeChunks(rawURL string, out io.Writer, bar io.Writer, size int64, chunks int, opts Options) error {
	if size <= 0 {
		return fmt.Errorf("invalid range download size %d", size)
	}
	body := make([]byte, int(size))
	ranges := splitByteRanges(size, chunks)
	progressWriter, closeProgress := concurrentProgressWriter(bar)

	var wg sync.WaitGroup
	errCh := make(chan error, len(ranges))
	for _, br := range ranges {
		br := br
		wg.Add(1)
		go func() {
			defer wg.Done()
			rangeHeader := fmt.Sprintf("bytes=%d-%d", br.Start, br.End)
			resp, err := requestWithOptions(http.MethodGet, rawURL, rangeHeader, opts)
			if err != nil {
				errCh <- fmt.Errorf("download range %s: %w", rangeHeader, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusPartialContent {
				errCh <- fmt.Errorf("download range %s returned status %d", rangeHeader, resp.StatusCode)
				return
			}
			dst := body[br.Start : br.End+1]
			n, err := io.ReadFull(resp.Body, dst)
			if err != nil {
				errCh <- fmt.Errorf("download range %s: %w", rangeHeader, err)
				return
			}
			if int64(n) != br.End-br.Start+1 {
				errCh <- fmt.Errorf("download range %s length mismatch", rangeHeader)
				return
			}
			if progressWriter != nil {
				_, _ = progressWriter.Write(dst[:n])
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			closeProgress()
			return err
		}
	}
	closeProgress()
	_, err := out.Write(body)
	if err == nil {
		if finisher, ok := bar.(progressFinisher); ok {
			finisher.Finish()
		}
	}
	return err
}

func requestWithOptions(method, rawURL, rangeHeader string, opts Options) (*http.Response, error) {
	client, err := newHTTPClient(opts)
	if err != nil {
		return nil, err
	}
	originalURL, err := urlpkgParse(rawURL)
	if err != nil {
		return nil, err
	}
	attempts := requestAttemptURLs(rawURL, originalURL, opts)
	var lastErr error
	for i, attemptURL := range attempts {
		req, err := http.NewRequest(method, attemptURL, nil)
		if err != nil {
			return nil, err
		}
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		if err := setAuthHeader(req, opts.DisableSSL); err != nil {
			return nil, err
		}
		if attemptURL != rawURL {
			verbosef("ghproxy rewrite: %s -> %s", rawURL, attemptURL)
		}
		if len(attempts) > 1 {
			verbosef("ghproxy attempt %d/%d: %s", i+1, len(attempts), attemptURL)
		}
		verbosef("request: %s %s", req.Method, req.URL.String())
		resp, err := httpDo(client, req)
		if err != nil {
			verbosef("request error: %v", err)
			lastErr = err
			if i < len(attempts)-1 {
				verbosef("ghproxy fallback: switching to next host")
				continue
			}
			return nil, err
		}
		verbosef("response: %s %s", req.URL.String(), resp.Status)
		return resp, nil
	}
	return nil, lastErr
}

func downloadProgressWriter(getbar func(size int64) io.Writer, size int64) io.Writer {
	if getbar == nil {
		return io.Discard
	}
	bar := getbar(size)
	if bar == nil {
		return io.Discard
	}
	return bar
}

func concurrentProgressWriter(bar io.Writer) (io.Writer, func()) {
	if bar == nil || bar == io.Discard {
		return io.Discard, func() {}
	}
	if p, ok := bar.(*progress.Progress); ok {
		writer := progress.NewConcurrentWriterWithInterval(p, 100*time.Millisecond)
		return writer, func() { _ = writer.Close() }
	}
	if closer, ok := bar.(io.Closer); ok {
		return &lockedWriter{writer: bar}, func() { _ = closer.Close() }
	}
	return &lockedWriter{writer: bar}, func() {}
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}

func isLocalFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func printProxyNotice(kind, proxyURL string) {
	if proxyURL == "" || proxyNoticeWriter == nil {
		return
	}
	ccolor.Fprintf(proxyNoticeWriter, " - Using <ylw>proxy_url for %s</>: %s\n", kind, proxyURL)
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

func isGitHubAPIRequest(u *url.URL) bool {
	return u != nil && strings.EqualFold(u.Host, "api.github.com")
}

func CacheFilePath(cacheDir, rawURL string) string {
	return CacheFilePathWithMeta(cacheDir, rawURL, CacheMeta{})
}

func CacheFilePathWithMeta(cacheDir, rawURL string, meta CacheMeta) string {
	if cacheDir == "" || rawURL == "" {
		return ""
	}
	u, _ := url.Parse(rawURL)
	fileName := cacheSourceFileName(rawURL, u)
	ext := archiveExt(fileName)
	if ext == "" {
		ext = ".bin"
	}
	name := strings.TrimSuffix(fileName, ext)
	if isGenericCacheName(name, ext) && meta.Name != "" {
		name = meta.Name
	}
	if name == "" {
		name = "download"
	}
	version := cacheVersion(rawURL, u, fileName, meta.Version)
	return filepath.Join(cacheDir, fmt.Sprintf("%s-%s-%s%s", safeCachePart(name), safeCachePart(version), shortURLHash(rawURL), ext))
}

func APICacheFilePath(cacheDir, rawURL string) string {
	if cacheDir == "" || rawURL == "" {
		return ""
	}
	u, _ := url.Parse(rawURL)
	name := apiCacheName(rawURL, u)
	return filepath.Join(cacheDir, fmt.Sprintf("%s-%s.json", safeCachePart(name), shortURLHash(rawURL)))
}

func cacheSourceFileName(rawURL string, u *url.URL) string {
	if u != nil && u.Path != "" {
		if base := path.Base(strings.TrimRight(u.Path, "/")); base != "." && base != "/" {
			return base
		}
	}
	base := path.Base(strings.TrimRight(rawURL, "/"))
	if idx := strings.IndexAny(base, "?#"); idx >= 0 {
		base = base[:idx]
	}
	if base == "" || base == "." || base == "/" {
		return "download"
	}
	return base
}

func archiveExt(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tar.zst", ".tar.br", ".tar.lz4"} {
		if strings.HasSuffix(lower, ext) {
			return name[len(name)-len(ext):]
		}
	}
	return path.Ext(name)
}

func cacheVersion(rawURL string, u *url.URL, fileName, fallback string) string {
	if u != nil {
		parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
		for i := 0; i+2 < len(parts); i++ {
			if parts[i] == "releases" && parts[i+1] == "download" {
				if tag, err := url.PathUnescape(parts[i+2]); err == nil && tag != "" {
					return trimVersionPrefix(tag)
				}
				return trimVersionPrefix(parts[i+2])
			}
		}
	}
	if version := versionFromText(fileName); version != "" {
		return trimVersionPrefix(version)
	}
	if version := versionFromText(rawURL); version != "" {
		return trimVersionPrefix(version)
	}
	if fallback != "" {
		return trimVersionPrefix(fallback)
	}
	return "unknown"
}

func isGenericCacheName(name, ext string) bool {
	if ext != ".bin" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "download", "latest", "release", "asset", "file":
		return true
	default:
		return false
	}
}

var cacheVersionPattern = regexp.MustCompile(`(?i)(?:^|[^0-9])v?(\d+(?:\.\d+)+(?:[-+][0-9A-Za-z.-]+)?)`)

func versionFromText(text string) string {
	match := cacheVersionPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func trimVersionPrefix(version string) string {
	return strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V")
}

func apiCacheName(rawURL string, u *url.URL) string {
	if u != nil && u.Host != "" {
		parts := []string{u.Host}
		for _, part := range strings.Split(strings.Trim(u.Path, "/"), "/") {
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, "-")
	}
	name := strings.TrimSpace(rawURL)
	if name == "" {
		return "api"
	}
	return name
}

func shortURLHash(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(sum[:])[:8]
}

func safeCachePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "unknown"
	}
	return out
}

func requestAttemptURLs(rawURL string, parsed *url.URL, opts Options) []string {
	if !opts.GhproxyEnabled {
		return []string{rawURL}
	}
	if parsed == nil {
		return []string{rawURL}
	}
	if !isGitHubDownloadRequest(parsed) && !(opts.GhproxySupportAPI && isGitHubAPIRequest(parsed)) {
		return []string{rawURL}
	}

	hosts := make([]string, 0, 1+len(opts.GhproxyFallbacks))
	if opts.GhproxyHostURL != "" {
		hosts = append(hosts, opts.GhproxyHostURL)
	}
	hosts = append(hosts, opts.GhproxyFallbacks...)
	if len(hosts) == 0 {
		return []string{rawURL}
	}

	attempts := make([]string, 0, len(hosts))
	seen := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		host = strings.TrimRight(strings.TrimSpace(host), "/")
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		attempts = append(attempts, host+"/"+rawURL)
	}
	if len(attempts) == 0 {
		return []string{rawURL}
	}
	return attempts
}

func resolvedAPICachePath(opts Options, rawURL string, parsed *url.URL) (string, bool) {
	if !opts.APICacheEnabled || !isProviderMetadataRequest(parsed) {
		return "", false
	}
	cacheDir := opts.APICacheDir
	if cacheDir == "" {
		return "", false
	}
	expanded, err := util.Expand(cacheDir)
	if err != nil {
		verbosef("api cache expand error: %v", err)
		return "", false
	}
	return APICacheFilePath(expanded, rawURL), true
}

func isProviderMetadataRequest(u *url.URL) bool {
	if u == nil {
		return false
	}
	switch {
	case isGitHubAPIRequest(u):
		return true
	case isGitLabAPIRequest(u):
		return true
	case isGiteaAPIRequest(u):
		return true
	case isSourceForgeFilesRequest(u):
		return true
	default:
		return false
	}
}

func isGitLabAPIRequest(u *url.URL) bool {
	if u == nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	return len(parts) >= 5 && parts[0] == "api" && parts[1] == "v4" && parts[2] == "projects" && parts[4] == "releases"
}

func isGiteaAPIRequest(u *url.URL) bool {
	if u == nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	return len(parts) >= 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "repos" && parts[5] == "releases"
}

func isSourceForgeFilesRequest(u *url.URL) bool {
	if u == nil || !strings.EqualFold(u.Host, "sourceforge.net") {
		return false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) < 3 || parts[0] != "projects" || parts[2] != "files" {
		return false
	}
	return !strings.EqualFold(parts[len(parts)-1], "download")
}

func loadAPICacheResponse(path string, cacheTime int) (*http.Response, bool, error) {
	if path == "" {
		return nil, false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if cacheTime > 0 && time.Since(info.ModTime()) > time.Duration(cacheTime)*time.Second {
		verbosef("api cache expired: %s", path)
		return nil, false, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Body:          io.NopCloser(strings.NewReader(string(body))),
		ContentLength: int64(len(body)),
		Header:        make(http.Header),
	}, true, nil
}

func storeAPICacheResponse(path string, resp *http.Response) (*http.Response, error) {
	if path == "" || resp == nil || resp.Body == nil {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(strings.NewReader(string(body)))
	resp.ContentLength = int64(len(body))
	return resp, nil
}

func isGitHubDownloadRequest(u *url.URL) bool {
	return u != nil && strings.EqualFold(u.Host, "github.com")
}

func urlpkgParse(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}
