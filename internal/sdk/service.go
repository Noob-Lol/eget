package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/inherelab/eget/internal/client"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/util"
)

type DownloaderFunc func(context.Context, DownloadRequest) (DownloadResult, error)

type Service struct {
	Config     *cfgpkg.File
	Store      Store
	IndexCache IndexCache
	ClientOpts client.Options
	GOOS       string
	GOARCH     string
	Now        func() time.Time
	Downloader DownloaderFunc

	OnIndexRefresh func(IndexRefreshEvent)
}

type InstallOptions struct {
	Force    bool
	Progress func(size int64) io.Writer
	OnStart  func(target string, version string, host string)
}

type InstallResult struct {
	Name    string
	Version string
	Path    string
	URL     string
	Cached  bool
	Resumed bool
}

type RemoveResult struct {
	Name    string
	Version string
	Path    string
	Missing bool
}

func (s Service) Install(ctx context.Context, rawTarget string, opts InstallOptions) (InstallResult, error) {
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return InstallResult{}, err
	}
	cfg, err := s.resolveConfig(target.Name)
	if err != nil {
		return InstallResult{}, err
	}
	version, file, err := s.resolveVersionAndFile(target, cfg)
	if err != nil {
		return InstallResult{}, err
	}
	vars := TemplateVars{Name: cfg.Name, Version: version, OS: cfg.OS, Arch: cfg.Arch, Ext: cfg.Ext}
	installPath, err := s.resolveInstallPath(cfg, vars)
	if err != nil {
		return InstallResult{}, err
	}
	if _, err := os.Stat(installPath); err == nil {
		if !opts.Force {
			return InstallResult{}, fmt.Errorf("sdk install path already exists: %s", installPath)
		}
		if err := s.ensureSafeSDKPath(installPath, cfg); err != nil {
			return InstallResult{}, err
		}
		if err := os.RemoveAll(installPath); err != nil {
			return InstallResult{}, err
		}
	} else if !os.IsNotExist(err) {
		return InstallResult{}, err
	}

	url := file.URL
	filename := file.Filename
	if url == "" {
		url, err = RenderTemplate(cfg.URLTemplate, vars)
		if err != nil {
			return InstallResult{}, err
		}
		filename = filepath.Base(url)
	}
	if filename == "" {
		filename = filepath.Base(url)
	}
	if opts.OnStart != nil {
		host := indexSourceHost(url)
		if host == "" {
			host = url
		}
		opts.OnStart(rawTarget, version, host)
	}
	download := s.Downloader
	if download == nil {
		download = DownloadArchive
	}
	downloadResult, err := download(ctx, DownloadRequest{
		URL:        url,
		CacheDir:   s.cacheDir(),
		SDK:        cfg.Name,
		Version:    version,
		Filename:   filename,
		ClientOpts: s.effectiveClientOptions(),
		Progress:   opts.Progress,
	})
	if err != nil {
		return InstallResult{}, err
	}

	tmpDir := filepath.Join(s.sdkRoot(cfg), ".eget-tmp", fmt.Sprintf("%s-%s-%d", cfg.Name, version, s.now().UnixNano()))
	if err := os.RemoveAll(tmpDir); err != nil {
		return InstallResult{}, err
	}
	defer os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return InstallResult{}, err
	}
	chooser, err := install.NewGlobChooser("*")
	if err != nil {
		return InstallResult{}, err
	}
	extractor := install.NewExtractor(filename, cfg.Name, chooser)
	direct, ok := extractor.(install.DirectAllExtractor)
	if !ok {
		return InstallResult{}, fmt.Errorf("sdk archive extractor does not support direct extraction")
	}
	data, err := os.ReadFile(downloadResult.Path)
	if err != nil {
		return InstallResult{}, err
	}
	if withOptions, ok := direct.(interface {
		ExtractAllToWithOptions([]byte, string, install.ArchiveExtractOptions) ([]string, error)
	}); ok {
		if _, err := withOptions.ExtractAllToWithOptions(data, tmpDir, install.ArchiveExtractOptions{StripComponents: cfg.StripComponents}); err != nil {
			return InstallResult{}, err
		}
	} else {
		if _, err := direct.ExtractAllTo(data, tmpDir); err != nil {
			return InstallResult{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		return InstallResult{}, err
	}
	if err := os.Rename(tmpDir, installPath); err != nil {
		return InstallResult{}, err
	}

	entry := InstalledEntry{
		Name:            cfg.Name,
		Version:         version,
		Path:            installPath,
		URL:             url,
		Filename:        filename,
		OS:              cfg.OS,
		Arch:            cfg.Arch,
		Ext:             cfg.Ext,
		InstalledAt:     s.now(),
		StripComponents: cfg.StripComponents,
	}
	if err := s.Store.Record(entry); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Name: cfg.Name, Version: version, Path: installPath, URL: url, Cached: downloadResult.FromCache, Resumed: downloadResult.Resumed}, nil
}

func (s Service) InstallMany(ctx context.Context, targets []string, opts InstallOptions) ([]InstallResult, error) {
	results := make([]InstallResult, 0, len(targets))
	for _, target := range targets {
		result, err := s.Install(ctx, target, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s Service) List(name string) ([]InstalledEntry, error) {
	return s.Store.List(name)
}

func (s Service) Remove(rawTarget string) (RemoveResult, error) {
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return RemoveResult{}, err
	}
	if target.Kind == VersionLatest {
		return RemoveResult{}, fmt.Errorf("sdk remove requires an explicit version")
	}
	cfg, err := s.resolveConfig(target.Name)
	if err != nil {
		return RemoveResult{}, err
	}
	entry, err := s.Store.Remove(cfg.Name, target.Version)
	if err != nil {
		return RemoveResult{}, err
	}
	if err := s.ensureSafeSDKPath(entry.Path, cfg); err != nil {
		_ = s.Store.Record(entry)
		return RemoveResult{}, err
	}
	err = os.RemoveAll(entry.Path)
	missing := false
	if os.IsNotExist(err) {
		missing = true
		err = nil
	}
	if err != nil {
		_ = s.Store.Record(entry)
		return RemoveResult{}, err
	}
	return RemoveResult{Name: cfg.Name, Version: target.Version, Path: entry.Path, Missing: missing}, nil
}

func (s Service) RefreshIndex(ctx context.Context, name string) (Index, error) {
	cfg, err := s.resolveConfig(name)
	if err != nil {
		return Index{}, err
	}
	if cfg.IndexURL == "" {
		return Index{}, fmt.Errorf("sdk %s index_url is not configured", cfg.Name)
	}
	s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshFetchStart, SDK: cfg.Name, URL: cfg.IndexURL})
	body, err := s.fetchIndex(ctx, cfg.IndexURL)
	if err != nil {
		if cached, loadErr := s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL); loadErr == nil {
			s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshCacheHit, SDK: cfg.Name, URL: cfg.IndexURL, Err: err, Versions: len(cached.Items), Files: countIndexFiles(cached)})
			return cached, nil
		}
		return Index{}, err
	}
	defer body.Close()
	s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshFetchDone, SDK: cfg.Name, URL: cfg.IndexURL})

	var index Index
	format := cfg.IndexFormat
	parser := cfg.IndexParser
	switch {
	case cfg.IndexParser != "":
		if format == "" {
			format = "json"
		}
		s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseStart, SDK: cfg.Name, URL: cfg.IndexURL, Format: format, Parser: parser})
		index, err = ParseJSONIndex(body, cfg.IndexParser, JSONParseOptions{SDK: cfg.Name, SourceURL: cfg.IndexURL, Now: s.Now})
	case cfg.IndexFormat == "json":
		if parser == "" {
			parser = cfg.Name + "-json"
		}
		s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseStart, SDK: cfg.Name, URL: cfg.IndexURL, Format: "json", Parser: parser})
		index, err = ParseJSONIndex(body, cfg.IndexParser, JSONParseOptions{SDK: cfg.Name, SourceURL: cfg.IndexURL, Now: s.Now})
	default:
		if format == "" {
			format = "html"
		}
		s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseStart, SDK: cfg.Name, URL: cfg.IndexURL, Format: format})
		index, err = ParseHTMLIndex(body, HTMLParseOptions{
			SDK:             cfg.Name,
			SourceURL:       cfg.IndexURL,
			IndexPathPrefix: cfg.IndexPathPrefix,
			FilenamePattern: cfg.FilenamePattern,
			URLTemplate:     cfg.URLTemplate,
			OS:              cfg.OS,
			Arch:            cfg.Arch,
			Ext:             cfg.Ext,
			Now:             s.Now,
		})
	}
	if err != nil {
		s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseFailed, SDK: cfg.Name, URL: cfg.IndexURL, Format: format, Parser: parser, Err: err})
		return Index{}, err
	}
	s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseDone, SDK: cfg.Name, URL: cfg.IndexURL, Format: format, Parser: parser, Versions: len(index.Items), Files: countIndexFiles(index)})
	if err := s.IndexCache.Save(index); err != nil {
		return Index{}, err
	}
	return index, nil
}

func (s Service) RefreshAllIndexes(ctx context.Context) ([]Index, error) {
	if s.Config == nil {
		return nil, nil
	}
	names := make([]string, 0, len(s.Config.SDK))
	for name, section := range s.Config.SDK {
		if section.IndexURL != nil && *section.IndexURL != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	indexes := make([]Index, 0, len(names))
	for _, name := range names {
		index, err := s.RefreshIndex(ctx, name)
		if err != nil {
			return nil, err
		}
		indexes = append(indexes, index)
	}
	return indexes, nil
}

func (s Service) ShowIndex(name string) (Index, error) {
	cfg, err := s.resolveConfig(name)
	if err != nil {
		return Index{}, err
	}
	return s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL)
}

func (s Service) ListIndexes() ([]CachedIndexInfo, error) {
	if s.Config == nil || len(s.Config.SDK) == 0 {
		return []CachedIndexInfo{}, nil
	}
	names := make([]string, 0, len(s.Config.SDK))
	for name, section := range s.Config.SDK {
		if section.IndexURL == nil || strings.TrimSpace(*section.IndexURL) == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	infos := make([]CachedIndexInfo, 0, len(names))
	for _, name := range names {
		cfg, err := s.resolveConfig(name)
		if err != nil {
			return nil, err
		}
		info := CachedIndexInfo{
			SDK:       cfg.Name,
			SourceURL: cfg.IndexURL,
			Path:      s.IndexCache.PathForSource(cfg.Name, cfg.IndexURL),
		}
		index, err := s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL)
		if err == nil {
			info.Versions = len(index.Items)
			info.SourceURL = index.SourceURL
			info.FetchedAt = index.FetchedAt
			info.Path = s.IndexCache.PathForSource(index.SDK, index.SourceURL)
			info.Cached = true
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (s Service) SearchIndex(name string, opts SearchOptions) ([]SearchResult, error) {
	if err := validateSearchSort(opts.Sort); err != nil {
		return nil, err
	}
	cfg, err := s.resolveConfig(name)
	if err != nil {
		return nil, err
	}
	index, err := s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL)
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0)
	for _, item := range index.Items {
		for _, file := range item.Files {
			result := SearchResult{
				SDK:      index.SDK,
				Version:  item.Version,
				Stable:   item.Stable,
				OS:       file.OS,
				Arch:     file.Arch,
				Ext:      file.Ext,
				Filename: file.Filename,
				URL:      file.URL,
			}
			matched, err := searchResultMatches(result, opts.Keywords)
			if err != nil {
				return nil, err
			}
			if matched {
				results = append(results, result)
			}
		}
	}
	sortSearchResults(results, opts.Sort)
	return limitSearchResults(results, opts.Number), nil
}

func (s Service) ClearIndex(name string) error {
	cfg, err := s.resolveConfig(name)
	if err != nil {
		return err
	}
	return s.IndexCache.ClearForSource(cfg.Name, cfg.IndexURL)
}

func (s Service) ClearAllIndexes() error {
	return s.IndexCache.ClearAll()
}

func searchResultMatches(result SearchResult, keywords []string) (bool, error) {
	keywords = normalizeSearchKeywords(keywords)
	fields := searchResultFields(result)
	haystack := strings.ToLower(strings.Join(fields, " "))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		matched, err := searchKeywordMatches(fields, haystack, keyword)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func searchResultFields(result SearchResult) []string {
	return []string{
		result.SDK,
		result.Version,
		fmt.Sprintf("%t", result.Stable),
		stabilityName(result.Stable),
		result.OS,
		result.Arch,
		result.Ext,
		result.Filename,
		result.URL,
	}
}

func searchKeywordMatches(fields []string, haystack, keyword string) (bool, error) {
	exclude := strings.HasPrefix(keyword, "^")
	if exclude {
		keyword = strings.TrimPrefix(keyword, "^")
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return true, nil
	}
	contains, err := searchKeywordContains(fields, haystack, keyword)
	if err != nil {
		return false, err
	}
	if exclude {
		return !contains, nil
	}
	return contains, nil
}

func searchKeywordContains(fields []string, haystack, keyword string) (bool, error) {
	if strings.HasPrefix(keyword, "REG:") {
		re, err := regexp.Compile(strings.TrimPrefix(keyword, "REG:"))
		if err != nil {
			return false, err
		}
		for _, field := range fields {
			if re.MatchString(field) {
				return true, nil
			}
		}
		return false, nil
	}
	return strings.Contains(haystack, strings.ToLower(keyword)), nil
}

func normalizeSearchKeywords(keywords []string) []string {
	normalized := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		normalized = append(normalized, strings.Fields(keyword)...)
	}
	return normalized
}

func validateSearchSort(sortValue string) error {
	switch strings.ToLower(strings.TrimSpace(sortValue)) {
	case "", "asc", "desc":
		return nil
	default:
		return fmt.Errorf("invalid sdk search sort %q", sortValue)
	}
}

func sortSearchResults(results []SearchResult, sortValue string) {
	switch strings.ToLower(strings.TrimSpace(sortValue)) {
	case "asc":
		sort.SliceStable(results, func(i, j int) bool {
			return compareVersion(results[i].Version, results[j].Version) < 0
		})
	case "desc":
		sort.SliceStable(results, func(i, j int) bool {
			return compareVersion(results[i].Version, results[j].Version) > 0
		})
	}
}

func limitSearchResults(results []SearchResult, number int) []SearchResult {
	if number <= 0 || len(results) <= number {
		return results
	}
	return results[:number]
}

func stabilityName(stable bool) string {
	if stable {
		return "stable"
	}
	return "prerelease"
}

func (s Service) emitIndexRefresh(event IndexRefreshEvent) {
	if s.OnIndexRefresh != nil {
		s.OnIndexRefresh(event)
	}
}

func countIndexFiles(index Index) int {
	total := 0
	for _, item := range index.Items {
		total += len(item.Files)
	}
	return total
}

func (s Service) fetchIndex(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	body, err := s.fetchIndexBytes(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

func (s Service) fetchIndexBytes(ctx context.Context, rawURL string) ([]byte, error) {
	clientOpts := s.effectiveClientOptions()
	httpClient, err := newDownloadHTTPClient(clientOpts)
	if err != nil {
		return nil, err
	}
	body, pagination, err := s.fetchIndexPage(ctx, httpClient, clientOpts, rawURL)
	if err != nil {
		return nil, err
	}
	if pagination.NextPage == 0 {
		return body, nil
	}

	var merged []json.RawMessage
	if err := json.Unmarshal(body, &merged); err != nil {
		return nil, fmt.Errorf("paginated sdk index response must be a json array: %w", err)
	}
	seenPages := map[int]bool{}
	for pagination.NextPage > 0 {
		nextPage := pagination.NextPage
		if seenPages[nextPage] {
			return nil, fmt.Errorf("sdk index pagination loop detected at page %d", nextPage)
		}
		seenPages[nextPage] = true
		nextURL, err := indexPageURL(rawURL, nextPage)
		if err != nil {
			return nil, err
		}
		body, pagination, err = s.fetchIndexPage(ctx, httpClient, clientOpts, nextURL)
		if err != nil {
			return nil, err
		}
		var pageItems []json.RawMessage
		if err := json.Unmarshal(body, &pageItems); err != nil {
			return nil, fmt.Errorf("paginated sdk index response must be a json array: %w", err)
		}
		merged = append(merged, pageItems...)
	}
	return json.Marshal(merged)
}

func (s Service) fetchIndexPage(ctx context.Context, httpClient *http.Client, clientOpts client.Options, rawURL string) ([]byte, indexPagination, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, indexPagination{}, err
	}
	userAgent := clientOpts.UserAgent
	if userAgent == "" {
		userAgent = client.DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, indexPagination{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, indexPagination{}, fmt.Errorf("sdk index request failed: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, indexPagination{}, err
	}
	pagination, err := parseIndexPagination(resp.Header.Get("X-Pagination"))
	if err != nil {
		return nil, indexPagination{}, err
	}
	return body, pagination, nil
}

type indexPagination struct {
	NextPage int `json:"next_page"`
}

func parseIndexPagination(header string) (indexPagination, error) {
	if strings.TrimSpace(header) == "" {
		return indexPagination{}, nil
	}
	var pagination indexPagination
	if err := json.Unmarshal([]byte(header), &pagination); err != nil {
		return indexPagination{}, fmt.Errorf("parse sdk index pagination header: %w", err)
	}
	return pagination, nil
}

func indexPageURL(rawURL string, page int) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("page", fmt.Sprint(page))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (s Service) resolveConfig(name string) (Config, error) {
	goos := s.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := s.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return ResolveConfig(s.Config, name, ResolveConfigOptions{GOOS: goos, GOARCH: goarch})
}

func (s Service) effectiveClientOptions() client.Options {
	opts := s.ClientOpts
	if opts.UserAgent == "" && s.Config != nil && s.Config.Global.UserAgent != nil {
		opts.UserAgent = *s.Config.Global.UserAgent
	}
	return opts
}

func (s Service) resolveVersionAndFile(target Target, cfg Config) (string, IndexFile, error) {
	if target.Kind == VersionExact && cfg.URLTemplate != "" {
		return target.Version, IndexFile{}, nil
	}
	index, err := s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL)
	if err != nil {
		return "", IndexFile{}, err
	}
	item, err := SelectVersion(index, target)
	if err != nil {
		return "", IndexFile{}, err
	}
	file, err := SelectFile(item, cfg.OS, cfg.Arch, cfg.Ext)
	if err != nil {
		return "", IndexFile{}, err
	}
	return item.Version, file, nil
}

func (s Service) resolveInstallPath(cfg Config, vars TemplateVars) (string, error) {
	rendered, err := RenderTemplate(cfg.TargetTemplate, vars)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(rendered) {
		return filepath.Clean(rendered), nil
	}
	return filepath.Join(s.sdkRoot(cfg), filepath.FromSlash(rendered)), nil
}

func (s Service) sdkRoot(cfg Config) string {
	root := "sdks"
	if cfg.SDKTarget != "" {
		root = cfg.SDKTarget
	}
	expanded, err := util.Expand(root)
	if err == nil && expanded != "" {
		root = expanded
	}
	return filepath.Clean(root)
}

func (s Service) cacheDir() string {
	if s.IndexCache.Dir != "" {
		return filepath.Dir(s.IndexCache.Dir)
	}
	return "."
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Service) ensureSafeSDKPath(targetPath string, cfg Config) error {
	root, err := filepath.Abs(s.sdkRoot(cfg))
	if err != nil {
		return err
	}
	target, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." || rel == "" || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("unsafe sdk path %q outside %q", targetPath, root)
	}
	return nil
}
