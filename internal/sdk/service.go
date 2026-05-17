package sdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/inherelab/eget/internal/client"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
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
}

type InstallOptions struct {
	Force bool
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
		ClientOpts: s.ClientOpts,
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
	body, err := s.fetchIndex(ctx, cfg.IndexURL)
	if err != nil {
		if cached, loadErr := s.IndexCache.Load(cfg.Name); loadErr == nil {
			return cached, nil
		}
		return Index{}, err
	}
	defer body.Close()

	var index Index
	switch {
	case cfg.IndexParser != "":
		index, err = ParseJSONIndex(body, cfg.IndexParser, JSONParseOptions{SDK: cfg.Name, SourceURL: cfg.IndexURL, Now: s.Now})
	case cfg.IndexFormat == "json":
		index, err = ParseJSONIndex(body, cfg.IndexParser, JSONParseOptions{SDK: cfg.Name, SourceURL: cfg.IndexURL, Now: s.Now})
	default:
		index, err = ParseHTMLIndex(body, HTMLParseOptions{
			SDK:             cfg.Name,
			SourceURL:       cfg.IndexURL,
			IndexPathPrefix: cfg.IndexPathPrefix,
			FilenamePattern: cfg.FilenamePattern,
			Now:             s.Now,
		})
	}
	if err != nil {
		return Index{}, err
	}
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
	return s.IndexCache.Load(cfg.Name)
}

func (s Service) ListIndexes() ([]CachedIndexInfo, error) {
	return s.IndexCache.List()
}

func (s Service) ClearIndex(name string) error {
	cfg, err := s.resolveConfig(name)
	if err != nil {
		return err
	}
	return s.IndexCache.Clear(cfg.Name)
}

func (s Service) ClearAllIndexes() error {
	return s.IndexCache.ClearAll()
}

func (s Service) fetchIndex(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	httpClient, err := newDownloadHTTPClient(s.ClientOpts)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("sdk index request failed: %d", resp.StatusCode)
	}
	return resp.Body, nil
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

func (s Service) resolveVersionAndFile(target Target, cfg Config) (string, IndexFile, error) {
	if target.Kind == VersionExact && cfg.URLTemplate != "" {
		return target.Version, IndexFile{}, nil
	}
	index, err := s.IndexCache.Load(cfg.Name)
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
	if cfg.SDKTarget != "" {
		return filepath.Clean(cfg.SDKTarget)
	}
	return filepath.Clean("sdks")
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
