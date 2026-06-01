package app

import (
	"fmt"
	"path/filepath"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/util"
)

func (s Service) resolveInstallRequest(target string, cli install.Options, preferCacheDir bool) (string, string, install.Options, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return "", "", install.Options{}, err
	}
	return s.resolveInstallRequestWithConfig(cfg, target, cli, preferCacheDir)
}

func (s Service) resolveInstallRequestWithConfig(cfg *cfgpkg.File, target string, cli install.Options, preferCacheDir bool) (string, string, install.Options, error) {
	if pkg, ok := cfg.Packages[target]; ok {
		repo := util.DerefString(pkg.Repo)
		if repo == "" {
			return "", "", install.Options{}, fmt.Errorf("package %q has no repo", target)
		}
		opts, err := s.resolveInstallOptionsWithConfig(cfg, repo, pkg, cli, preferCacheDir)
		if err != nil {
			return "", "", install.Options{}, err
		}
		return repo, target, opts, nil
	}

	pkg := packageSectionForRepoTarget(cfg, target)
	opts, err := s.resolveInstallOptionsWithConfig(cfg, target, pkg, cli, preferCacheDir)
	if err != nil {
		return "", "", install.Options{}, err
	}
	return target, installRecordTarget(target, opts), opts, nil
}

func installRecordTarget(target string, opts install.Options) string {
	if opts.Name != "" {
		return opts.Name
	}
	if normalized, err := install.NormalizeRepoTarget(target); err == nil {
		return repoName(normalized)
	}
	return target
}

func packageSectionForRepoTarget(cfg *cfgpkg.File, target string) cfgpkg.Section {
	targetRepo, err := install.NormalizeRepoTarget(target)
	if err != nil {
		return cfgpkg.Section{}
	}
	for _, pkg := range cfg.Packages {
		repo := util.DerefString(pkg.Repo)
		if repo == "" {
			continue
		}
		normalized, err := install.NormalizeRepoTarget(repo)
		if err != nil {
			continue
		}
		if normalized == targetRepo {
			return pkg
		}
	}
	return cfgpkg.Section{}
}

func (s Service) resolveInstallOptions(target string, cli install.Options, preferCacheDir bool) (install.Options, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return install.Options{}, err
	}
	return s.resolveInstallOptionsWithConfig(cfg, target, cfgpkg.Section{}, cli, preferCacheDir)
}

func (s Service) resolveInstallOptionsWithConfig(cfg *cfgpkg.File, target string, pkg cfgpkg.Section, cli install.Options, preferCacheDir bool) (install.Options, error) {
	repoKey := ""
	if repo, err := install.NormalizeRepoTarget(target); err == nil {
		repoKey = repo
	}

	merged := cfgpkg.MergeInstallOptions(cfg.Global, cfg.Repos[repoKey], pkg, cfgpkg.CLIOverrides{
		ExtractAll:       boolOpt(cli.All),
		AssetFilters:     stringsOpt(cli.Asset),
		CacheDir:         stringOpt(cli.CacheDir),
		ProxyURL:         stringOpt(cli.ProxyURL),
		DownloadOnly:     boolOpt(cli.DownloadOnly),
		File:             stringOpt(cli.ExtractFile),
		IsGUI:            boolOpt(cli.IsGUI),
		InstallMode:      stringOpt(cli.InstallMode),
		Name:             stringOpt(cli.Name),
		Quiet:            boolOpt(cli.Quiet),
		RenameFiles:      mapOpt(cli.RenameFiles),
		ShowHash:         boolOpt(cli.Hash),
		StripComponents:  intOpt(cli.StripComponents, cli.StripComponents > 0),
		Source:           boolOpt(cli.Source),
		SourcePath:       stringOpt(cli.SourcePath),
		System:           stringOpt(cli.System),
		Tag:              stringOpt(cli.Tag),
		Target:           stringOpt(cli.Output),
		UpgradeOnly:      boolOpt(cli.UpgradeOnly),
		Verify:           stringOpt(cli.Verify),
		DisableSSL:       boolOpt(cli.DisableSSL),
		ChunkConcurrency: intOpt(cli.ChunkConcurrency, cli.ChunkConcurrencySet),
	})

	targetDir, err := expandPath(merged.Target)
	if err != nil {
		return install.Options{}, err
	}
	cacheDir, err := expandPath(merged.CacheDir)
	if err != nil {
		return install.Options{}, err
	}
	guiTarget, err := expandPath(merged.GuiTarget)
	if err != nil {
		return install.Options{}, err
	}
	sys7zPath, err := expandPath(merged.Sys7zPath)
	if err != nil {
		return install.Options{}, err
	}
	apiCacheDir := ""
	if cacheDir != "" {
		apiCacheDir = filepath.Join(cacheDir, "api-cache")
	}

	output := targetDir
	if preferCacheDir && cli.Output == "" && cacheDir != "" {
		output = cacheDir
	}

	apiCacheEnabled := false
	if cfg.ApiCache.Enable != nil {
		apiCacheEnabled = *cfg.ApiCache.Enable
	}
	apiCacheTime := 0
	if cfg.ApiCache.CacheTime != nil {
		apiCacheTime = *cfg.ApiCache.CacheTime
	}
	ghproxyEnabled := false
	if cfg.Ghproxy.Enable != nil {
		ghproxyEnabled = *cfg.Ghproxy.Enable
	}
	ghproxyHostURL := util.DerefString(cfg.Ghproxy.HostURL)
	ghproxySupportAPI := false
	if cfg.Ghproxy.SupportAPI != nil {
		ghproxySupportAPI = *cfg.Ghproxy.SupportAPI
	}
	batchConcurrency := 0
	if cli.BatchConcurrencySet || cli.BatchConcurrency > 0 {
		batchConcurrency = cli.BatchConcurrency
	} else if cfg.Global.BatchConcurrency != nil {
		batchConcurrency = *cfg.Global.BatchConcurrency
	}

	cacheName := merged.Name
	if cli.CacheName != "" {
		cacheName = cli.CacheName
	}
	cacheVersion := merged.Tag
	if cli.CacheVersion != "" {
		cacheVersion = cli.CacheVersion
	}

	return install.Options{
		Tag:                 merged.Tag,
		Operation:           cli.Operation,
		CurrentVersion:      cli.CurrentVersion,
		TargetVersion:       cli.TargetVersion,
		Name:                merged.Name,
		Source:              merged.Source,
		SourcePath:          merged.SourcePath,
		Sys7zPath:           sys7zPath,
		Output:              output,
		OutputExplicit:      cli.Output != "",
		GuiTarget:           guiTarget,
		IsGUI:               merged.IsGUI,
		InstallMode:         merged.InstallMode,
		CacheDir:            cacheDir,
		CacheName:           cacheName,
		CacheVersion:        cacheVersion,
		ProxyURL:            merged.ProxyURL,
		UserAgent:           merged.UserAgent,
		APICacheEnabled:     apiCacheEnabled,
		APICacheDir:         apiCacheDir,
		APICacheTime:        apiCacheTime,
		GhproxyEnabled:      ghproxyEnabled,
		GhproxyHostURL:      ghproxyHostURL,
		GhproxySupportAPI:   ghproxySupportAPI,
		GhproxyFallbacks:    append([]string(nil), cfg.Ghproxy.Fallbacks...),
		System:              merged.System,
		ExtractFile:         merged.File,
		All:                 merged.ExtractAll,
		StripComponents:     merged.StripComponents,
		Quiet:               merged.Quiet,
		DownloadOnly:        merged.DownloadOnly,
		FallbackVersions:    cli.FallbackVersions,
		ChunkConcurrency:    merged.ChunkConcurrency,
		BatchConcurrency:    batchConcurrency,
		ChunkConcurrencySet: true,
		BatchConcurrencySet: true,
		UpgradeOnly:         merged.UpgradeOnly,
		Asset:               append([]string(nil), merged.AssetFilters...),
		RenameFiles:         util.CloneStringMap(merged.RenameFiles),
		Hash:                merged.ShowHash,
		Verify:              merged.Verify,
		URLTemplate: install.URLTemplateOptions{
			URLTemplate:         merged.URLTemplate,
			LatestURL:           merged.LatestURL,
			LatestFormat:        merged.LatestFormat,
			LatestJSONPath:      merged.LatestJSONPath,
			VersionRegex:        merged.VersionRegex,
			OSMap:               util.CloneStringMap(merged.OSMap),
			ArchMap:             util.CloneStringMap(merged.ArchMap),
			ExtMap:              util.CloneStringMap(merged.ExtMap),
			LibcMap:             util.CloneStringMap(merged.LibcMap),
			ChecksumURLTemplate: merged.ChecksumURLTemplate,
			ChecksumFormat:      merged.ChecksumFormat,
			ChecksumJSONPath:    merged.ChecksumJSONPath,
			ChecksumRegex:       merged.ChecksumRegex,
			InstallAction:       merged.InstallAction,
			InstallArgs:         append([]string(nil), merged.InstallArgs...),
		},
		DisableSSL: merged.DisableSSL,
	}, nil
}
