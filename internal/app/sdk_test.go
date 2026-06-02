package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/sdk"
	"github.com/inherelab/eget/internal/util"
)

func TestNewDefaultSDKServiceUsesConfigPathsAndNetworkOptions(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	t.Setenv("EGET_CONFIG", configPath)

	cacheDir := filepath.Join(tmp, "cache")
	proxyURL := "http://127.0.0.1:7890"
	userAgent := "custom-agent/1.0"
	apiCacheEnable := true
	apiCacheTime := 180
	disableSSL := true
	chunkConcurrency := 3
	ghproxyEnable := true
	ghproxyHost := "https://gh.example.com"
	ghproxySupportAPI := false
	cacheMirrorEnable := true
	cacheMirrorURL := "http://mirror.local:8686/"
	cacheMirrorTimeout := 4
	cacheMirrorFallback := false
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &cacheDir
	cfg.Global.ProxyURL = &proxyURL
	cfg.Global.UserAgent = &userAgent
	cfg.Global.DisableSSL = &disableSSL
	cfg.Global.ChunkConcurrency = &chunkConcurrency
	cfg.ApiCache.Enable = &apiCacheEnable
	cfg.ApiCache.CacheTime = &apiCacheTime
	cfg.Ghproxy.Enable = &ghproxyEnable
	cfg.Ghproxy.HostURL = &ghproxyHost
	cfg.Ghproxy.SupportAPI = &ghproxySupportAPI
	cfg.Ghproxy.Fallbacks = []string{"https://gh2.example.com"}
	cfg.CacheMirror.Enable = &cacheMirrorEnable
	cfg.CacheMirror.URL = &cacheMirrorURL
	cfg.CacheMirror.Timeout = &cacheMirrorTimeout
	cfg.CacheMirror.Fallback = &cacheMirrorFallback

	service, err := NewDefaultSDKService(cfg)
	assert.NoErr(t, err)

	wantStorePath, err := sdk.DefaultStorePath()
	assert.NoErr(t, err)
	assert.Eq(t, cfg, service.Config)
	assert.Eq(t, filepath.Join(cacheDir, "sdk-index"), service.IndexCache.Dir)
	assert.Eq(t, wantStorePath, service.Store.Path)
	assert.Eq(t, proxyURL, service.ClientOpts.ProxyURL)
	assert.Eq(t, userAgent, service.ClientOpts.UserAgent)
	assert.True(t, service.ClientOpts.APICacheEnabled)
	assert.Eq(t, filepath.Join(cacheDir, "api-cache"), service.ClientOpts.APICacheDir)
	assert.Eq(t, apiCacheTime, service.ClientOpts.APICacheTime)
	assert.True(t, service.ClientOpts.DisableSSL)
	assert.Eq(t, chunkConcurrency, service.ClientOpts.ChunkConcurrency)
	assert.True(t, service.ClientOpts.GhproxyEnabled)
	assert.Eq(t, ghproxyHost, service.ClientOpts.GhproxyHostURL)
	assert.False(t, service.ClientOpts.GhproxySupportAPI)
	assert.Eq(t, []string{"https://gh2.example.com"}, service.ClientOpts.GhproxyFallbacks)
	assert.True(t, service.CacheMirror.Enable)
	assert.Eq(t, "http://mirror.local:8686", service.CacheMirror.URL)
	assert.Eq(t, 4*time.Second, service.CacheMirror.Timeout)
	assert.False(t, service.CacheMirror.Fallback)
}

func TestNewDefaultSDKServiceLoadsConfigWhenNil(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	t.Setenv("EGET_CONFIG", configPath)

	cacheDir := filepath.Join(tmp, "cache")
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = util.StringPtr(cacheDir)
	assert.NoErr(t, cfgpkg.Save(configPath, cfg))

	service, err := NewDefaultSDKService(nil)
	assert.NoErr(t, err)

	assert.Eq(t, filepath.Join(cacheDir, "sdk-index"), service.IndexCache.Dir)
}
