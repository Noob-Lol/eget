package cli

import (
	"path/filepath"
	"strings"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/util"
)

func installOptionsFromInstall(opts *InstallOptions) install.Options {
	return install.Options{
		Tag:                 opts.Tag,
		Name:                opts.Name,
		Source:              opts.Source,
		Output:              opts.To,
		OutputExplicit:      opts.To != "",
		System:              opts.System,
		ExtractFile:         opts.File,
		All:                 opts.All,
		IsGUI:               opts.GUI,
		Quiet:               opts.Quiet,
		FallbackVersions:    opts.FallbackVersions,
		ChunkConcurrency:    opts.ChunkConcurrency,
		BatchConcurrency:    opts.BatchConcurrency,
		ChunkConcurrencySet: opts.ChunkConcurrency >= 0,
		BatchConcurrencySet: opts.BatchConcurrency >= 0,
		Asset:               splitAssetFilters(opts.Asset),
		RenameFiles:         splitRenameFiles(opts.Rename),
	}
}

func applyGlobalNetworkConfig(opts *install.Options, cfg *cfgpkg.File) {
	if opts == nil || cfg == nil {
		return
	}
	if cfg.ApiCache.Enable != nil {
		opts.APICacheEnabled = *cfg.ApiCache.Enable
	}
	if cfg.ApiCache.CacheTime != nil {
		opts.APICacheTime = *cfg.ApiCache.CacheTime
	}
	if cfg.Global.CacheDir != nil {
		if cacheDir, err := util.Expand(*cfg.Global.CacheDir); err == nil && cacheDir != "" {
			opts.CacheDir = cacheDir
			opts.APICacheDir = filepath.Join(cacheDir, "api-cache")
		}
	}
	if cfg.Global.ProxyURL != nil {
		opts.ProxyURL = *cfg.Global.ProxyURL
	}
	if cfg.Ghproxy.Enable != nil {
		opts.GhproxyEnabled = *cfg.Ghproxy.Enable
	}
	if cfg.Ghproxy.HostURL != nil {
		opts.GhproxyHostURL = *cfg.Ghproxy.HostURL
	}
	if cfg.Ghproxy.SupportAPI != nil {
		opts.GhproxySupportAPI = *cfg.Ghproxy.SupportAPI
	}
	if len(cfg.Ghproxy.Fallbacks) > 0 {
		opts.GhproxyFallbacks = append([]string(nil), cfg.Ghproxy.Fallbacks...)
	}
}

func installOptionsFromDownload(opts *DownloadOptions) install.Options {
	base := installOptionsFromInstall(&InstallOptions{
		Tag:    opts.Tag,
		System: opts.System,
		To:     opts.To,
		File:   opts.File,
		Asset:  opts.Asset,
		Rename: opts.Rename,
		Source: opts.Source,
		All:    opts.All,
		Quiet:  opts.Quiet,
	})
	base.FallbackVersions = opts.FallbackVersions
	base.ChunkConcurrency = opts.ChunkConcurrency
	base.ChunkConcurrencySet = opts.ChunkConcurrency >= 0
	if hasMultipleFilePatterns(opts.File) {
		base.All = true
	}
	base.DownloadOnly = opts.File == "" && !opts.All
	return base
}

func installOptionsFromAdd(opts *AddOptions) install.Options {
	return install.Options{
		Tag:                 opts.Tag,
		Source:              opts.Source,
		Output:              opts.To,
		OutputExplicit:      opts.To != "",
		System:              opts.System,
		ExtractFile:         opts.File,
		All:                 opts.All,
		IsGUI:               opts.GUI,
		Quiet:               opts.Quiet,
		ChunkConcurrency:    opts.ChunkConcurrency,
		ChunkConcurrencySet: opts.ChunkConcurrency >= 0,
		Asset:               splitAssetFilters(opts.Asset),
		RenameFiles:         splitRenameFiles(opts.Rename),
	}
}

func installOptionsFromUpdate(opts *UpdateOptions) install.Options {
	return install.Options{
		Tag:                 opts.Tag,
		Source:              opts.Source,
		Output:              opts.To,
		System:              opts.System,
		Quiet:               opts.Quiet,
		ChunkConcurrency:    opts.ChunkConcurrency,
		BatchConcurrency:    opts.BatchConcurrency,
		ChunkConcurrencySet: opts.ChunkConcurrency >= 0,
		BatchConcurrencySet: opts.BatchConcurrency >= 0,
		Asset:               splitAssetFilters(opts.Asset),
	}
}

func splitAssetFilters(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func splitRenameFiles(value string) map[string]string {
	if value == "" {
		return nil
	}
	items := map[string]string{}
	for _, part := range strings.Split(value, ",") {
		from, to, ok := strings.Cut(part, "=")
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		if !ok || from == "" || to == "" {
			continue
		}
		items[from] = to
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func hasMultipleFilePatterns(value string) bool {
	parts := strings.Split(value, ",")
	count := 0
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			count++
			if count > 1 {
				return true
			}
		}
	}
	return false
}
