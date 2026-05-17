package app

import (
	"path/filepath"

	"github.com/inherelab/eget/internal/client"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/sdk"
	"github.com/inherelab/eget/internal/util"
)

func NewDefaultSDKService(cfg *cfgpkg.File) (sdk.Service, error) {
	if cfg == nil {
		loaded, err := cfgpkg.Load()
		if err != nil {
			return sdk.Service{}, err
		}
		cfg = loaded
	}

	storePath, err := sdk.DefaultStorePath()
	if err != nil {
		return sdk.Service{}, err
	}

	clientOpts := sdkClientOptionsFromConfig(cfg)
	cacheDir := clientOpts.APICacheDir
	if cfg != nil && cfg.Global.CacheDir != nil {
		if expanded, err := util.Expand(*cfg.Global.CacheDir); err == nil && expanded != "" {
			cacheDir = expanded
		}
	}
	if cacheDir == "" {
		cacheDir = filepath.Join(".", "cache")
	}

	return sdk.Service{
		Config:     cfg,
		Store:      sdk.Store{Path: storePath},
		IndexCache: sdk.IndexCache{Dir: filepath.Join(cacheDir, "sdk-index")},
		ClientOpts: clientOpts,
	}, nil
}

func sdkClientOptionsFromConfig(cfg *cfgpkg.File) client.Options {
	opts := client.Options{}
	if cfg == nil {
		return opts
	}
	if cfg.Global.ProxyURL != nil {
		opts.ProxyURL = *cfg.Global.ProxyURL
	}
	if cfg.Global.DisableSSL != nil {
		opts.DisableSSL = *cfg.Global.DisableSSL
	}
	if cfg.Global.ChunkConcurrency != nil {
		opts.ChunkConcurrency = *cfg.Global.ChunkConcurrency
	}
	if cfg.Global.CacheDir != nil {
		if cacheDir, err := util.Expand(*cfg.Global.CacheDir); err == nil && cacheDir != "" {
			opts.APICacheDir = filepath.Join(cacheDir, "api-cache")
		}
	}
	if cfg.ApiCache.Enable != nil {
		opts.APICacheEnabled = *cfg.ApiCache.Enable
	}
	if cfg.ApiCache.CacheTime != nil {
		opts.APICacheTime = *cfg.ApiCache.CacheTime
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
	return opts
}
