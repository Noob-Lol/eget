package app

import (
	"fmt"
	"strings"

	"github.com/inherelab/eget/internal/client"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	forge "github.com/inherelab/eget/internal/source/forge"
	"github.com/inherelab/eget/internal/source/sourceforge"
	"github.com/inherelab/eget/internal/util"
)

type ConfigService struct {
	ConfigPath   string
	Load         func() (*cfgpkg.File, error)
	Save         func(path string, file *cfgpkg.File) error
	RepoMetadata func(repo string) (RepoMetadata, error)
}

type RepoMetadata struct {
	Desc     string
	Homepage string
	RepoURL  string
}

type ConfigInfoResult struct {
	Path   string
	Exists bool
}

func (s ConfigService) AddPackage(repo, name string, opts install.Options) error {
	cfg, err := s.load()
	if err != nil {
		return err
	}

	repo, name, opts, err = ResolvePackageConfig(repo, name, opts)
	if err != nil {
		return err
	}

	if cfg.Packages == nil {
		cfg.Packages = make(map[string]cfgpkg.Section)
	}
	section := sectionFromInstallOptions(repo, name, opts)
	if section.Desc == nil || *section.Desc == "" {
		if meta, ok := s.repoMetadata(repo); ok && meta.Desc != "" {
			section.Desc = util.StringPtr(meta.Desc)
		}
	}
	cfg.Packages[name] = section
	return s.save(cfg)
}

func ResolvePackageName(repo, name string) (string, error) {
	_, resolvedName, _, err := ResolvePackageConfig(repo, name, install.Options{})
	return resolvedName, err
}

func ResolvePackageConfig(repo, name string, opts install.Options) (string, string, install.Options, error) {
	if sfTarget, sfErr := sourceforge.ParseTarget(repo); sfErr == nil {
		repo = sfTarget.Normalized
		if opts.SourcePath == "" {
			opts.SourcePath = sfTarget.Path
		}
		if name == "" {
			name = sfTarget.Project
		}
	}

	if forgeTarget, forgeErr := forge.ParseTarget(repo); forgeErr == nil {
		repo = forgeTarget.Normalized
		if name == "" {
			name = forgeTarget.Project
		}
	}

	if name == "" {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", install.Options{}, fmt.Errorf("invalid repo %q", repo)
		}
		name = parts[1]
	}
	return repo, name, opts, nil
}

func (s ConfigService) ConfigInfo() (ConfigInfoResult, error) {
	path := s.ConfigPath
	if path == "" {
		resolved, err := cfgpkg.ResolveConfigPath()
		if err != nil {
			if cfgpkg.IsNotExist(err) {
				return ConfigInfoResult{Exists: false}, nil
			}
			return ConfigInfoResult{}, err
		}
		path = resolved
	}
	_, err := cfgpkg.LoadFile(path)
	if err != nil {
		if cfgpkg.IsNotExist(err) {
			return ConfigInfoResult{Path: path, Exists: false}, nil
		}
		return ConfigInfoResult{}, err
	}
	return ConfigInfoResult{Path: path, Exists: true}, nil
}

func (s ConfigService) ConfigInit() (string, error) {
	path := s.ConfigPath
	if path == "" {
		resolved, err := cfgpkg.ResolveWritablePath()
		if err != nil {
			return "", err
		}
		path = resolved
	}

	file := cfgpkg.NewFile()
	target := "~/.local/bin"
	sdkTarget := "~/.local/sdks"
	cacheDir := "~/.cache/eget"
	proxyURL := ""
	userAgent := client.DefaultUserAgent
	empty := ""
	sys7zPath := ""
	apiCacheEnable := false
	apiCacheTime := 300
	ghproxyEnable := false
	ghproxyHostURL := ""
	ghproxySupportAPI := true
	chunkConcurrency := 0
	batchConcurrency := 0
	file.Global.Target = &target
	file.Global.SDKTarget = &sdkTarget
	file.Global.CacheDir = &cacheDir
	file.Global.ProxyURL = &proxyURL
	file.Global.UserAgent = &userAgent
	file.Global.System = &empty
	file.Global.Sys7zPath = &sys7zPath
	file.Global.ChunkConcurrency = &chunkConcurrency
	file.Global.BatchConcurrency = &batchConcurrency
	file.ApiCache.Enable = &apiCacheEnable
	file.ApiCache.CacheTime = &apiCacheTime
	file.Ghproxy.Enable = &ghproxyEnable
	file.Ghproxy.HostURL = &ghproxyHostURL
	file.Ghproxy.SupportAPI = &ghproxySupportAPI
	file.Ghproxy.Fallbacks = []string{}
	if err := cfgpkg.Save(path, file); err != nil {
		return "", err
	}
	return path, nil
}

func (s ConfigService) ConfigList() (*cfgpkg.File, error) {
	return s.load()
}

func (s ConfigService) ConfigGet(key string) (any, error) {
	cfg, err := s.load()
	if err != nil {
		return nil, err
	}

	value, ok := cfgpkg.GetByPath(cfg, key)
	if !ok {
		return nil, fmt.Errorf("unsupported config key %q", key)
	}
	return value, nil
}

func (s ConfigService) ConfigSet(key, value string) error {
	cfg, err := s.load()
	if err != nil {
		return err
	}

	// cast: packages.<name>.asset -> packages.<name>.asset_filters
	if strings.HasPrefix(key, "packages.") && strings.HasSuffix(key, ".asset") {
		key = key + "_filters"
	}
	if err := cfgpkg.SetByPath(cfg, key, value); err != nil {
		return err
	}
	return s.save(cfg)
}

func (s ConfigService) load() (*cfgpkg.File, error) {
	if s.Load != nil {
		return s.Load()
	}
	return cfgpkg.Load()
}

func (s ConfigService) save(file *cfgpkg.File) error {
	path := s.ConfigPath
	if path == "" {
		resolved, err := cfgpkg.ResolveWritablePath()
		if err != nil {
			return err
		}
		path = resolved
	}
	if s.Save != nil {
		return s.Save(path, file)
	}
	return cfgpkg.Save(path, file)
}

func sectionFromInstallOptions(repo, name string, opts install.Options) cfgpkg.Section {
	section := cfgpkg.Section{
		AssetFilters: append([]string(nil), opts.Asset...),
	}
	section.Repo = util.StringPtr(repo)
	section.Name = util.StringPtr(name)
	if opts.Output != "" {
		section.Target = util.StringPtr(opts.Output)
	}
	if opts.CacheDir != "" {
		section.CacheDir = util.StringPtr(opts.CacheDir)
	}
	if opts.System != "" {
		section.System = util.StringPtr(opts.System)
	}
	if opts.ExtractFile != "" {
		section.File = util.StringPtr(opts.ExtractFile)
	}
	if len(opts.RenameFiles) > 0 {
		section.RenameFiles = cloneStringMap(opts.RenameFiles)
	}
	if opts.Tag != "" {
		section.Tag = util.StringPtr(opts.Tag)
	}
	if opts.Verify != "" {
		section.Verify = util.StringPtr(opts.Verify)
	}
	if opts.Source {
		section.Source = util.BoolPtr(true)
	}
	if opts.SourcePath != "" {
		section.SourcePath = util.StringPtr(opts.SourcePath)
	}
	if opts.DisableSSL {
		section.DisableSSL = util.BoolPtr(true)
	}
	if opts.ChunkConcurrencySet || opts.ChunkConcurrency > 0 {
		section.ChunkConcurrency = &opts.ChunkConcurrency
	}
	if opts.All {
		section.ExtractAll = util.BoolPtr(true)
	}
	if opts.StripComponents > 0 {
		section.StripComponents = &opts.StripComponents
	}
	if opts.IsGUI {
		section.IsGUI = util.BoolPtr(true)
	}
	return section
}

func (s ConfigService) repoMetadata(repo string) (RepoMetadata, bool) {
	if s.RepoMetadata == nil {
		return RepoMetadata{}, false
	}
	meta, err := s.RepoMetadata(repo)
	if err != nil {
		return RepoMetadata{}, false
	}
	return meta, true
}
