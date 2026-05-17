package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestResolveConfigPathPrefersEnv(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "env.toml")
	homePath := filepath.Join(tmp, "home")

	writeTestFile(t, envPath, "title = 'env'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir: homePath,
		GOOS:    runtime.GOOS,
		LookupEnv: func(key string) (string, bool) {
			if key == "EGET_CONFIG" {
				return envPath, true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}

	if path != envPath {
		t.Fatalf("expected env path %q, got %q", envPath, path)
	}
}

func TestResolveConfigPathFallsBackToDotfile(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	dotfile := filepath.Join(homePath, ".eget.toml")

	writeTestFile(t, dotfile, "title = 'home'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir:   homePath,
		GOOS:      runtime.GOOS,
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}

	if path != dotfile {
		t.Fatalf("expected dotfile path %q, got %q", dotfile, path)
	}
}

func TestResolveConfigPathFallsBackToOSConfigDir(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")

	testCases := []struct {
		name     string
		goos     string
		envKey   string
		envValue string
		wantPath string
	}{
		{
			name:     "xdg",
			goos:     "linux",
			envKey:   "XDG_CONFIG_HOME",
			envValue: filepath.Join(tmp, "xdg"),
			wantPath: filepath.Join(tmp, "xdg", "eget", "eget.toml"),
		},
		{
			name:     "windows uses xdg env when set",
			goos:     "windows",
			envKey:   "XDG_CONFIG_HOME",
			envValue: filepath.Join(tmp, "xdg-win"),
			wantPath: filepath.Join(tmp, "xdg-win", "eget", "eget.toml"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			writeTestFile(t, tc.wantPath, "title = 'os'\n")

			path, err := resolveConfigPath(pathOptions{
				HomeDir: homePath,
				GOOS:    tc.goos,
				LookupEnv: func(key string) (string, bool) {
					if key == tc.envKey {
						return tc.envValue, true
					}
					return "", false
				},
			})
			if err != nil {
				t.Fatalf("resolve path: %v", err)
			}

			if path != tc.wantPath {
				t.Fatalf("expected os config path %q, got %q", tc.wantPath, path)
			}
		})
	}
}

func TestResolveConfigPathSkipsDotfileWhenEnvPathMissing(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	dotfile := filepath.Join(homePath, ".eget.toml")
	fallbackPath := filepath.Join(tmp, "xdg", "eget", "eget.toml")

	writeTestFile(t, dotfile, "title = 'home'\n")
	writeTestFile(t, fallbackPath, "title = 'fallback'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir: homePath,
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			switch key {
			case "EGET_CONFIG":
				return filepath.Join(tmp, "missing.toml"), true
			case "XDG_CONFIG_HOME":
				return filepath.Join(tmp, "xdg"), true
			default:
				return "", false
			}
		},
	})
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}

	if path != fallbackPath {
		t.Fatalf("expected fallback path %q when env config is missing, got %q", fallbackPath, path)
	}
}

func TestResolveWritablePathDefaultsToOSConfigDir(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	xdgHome := filepath.Join(tmp, "xdg")
	wantPath := filepath.Join(xdgHome, "eget", "eget.toml")

	path, err := resolveWritablePath(pathOptions{
		HomeDir: homePath,
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			if key == "XDG_CONFIG_HOME" {
				return xdgHome, true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("resolve writable path: %v", err)
	}

	if path != wantPath {
		t.Fatalf("expected writable path %q, got %q", wantPath, path)
	}
}

func TestResolveWritablePathDefaultsToHomeConfigDirOnWindows(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	wantPath := filepath.Join(homePath, ".config", "eget", "eget.toml")

	path, err := resolveWritablePath(pathOptions{
		HomeDir:   homePath,
		GOOS:      "windows",
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("resolve writable path: %v", err)
	}

	if path != wantPath {
		t.Fatalf("expected writable path %q, got %q", wantPath, path)
	}
}

func TestLoadFileSupportsLegacyRepoSections(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	writeTestFile(t, configPath, `
[global]
target = "~/bin"
gui_target = "~/Applications"
quiet = true
github_token = "token"

["owner/repo"]
asset_filters = ["linux", "!arm"]
download_only = true
extract_all = true
is_gui = true
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load file: %v", err)
	}

	if cfg.Global.Target == nil || *cfg.Global.Target != "~/bin" {
		t.Fatalf("expected global target to be loaded, got %#v", cfg.Global.Target)
	}
	if cfg.Global.GuiTarget == nil || *cfg.Global.GuiTarget != "~/Applications" {
		t.Fatalf("expected global gui_target to be loaded, got %#v", cfg.Global.GuiTarget)
	}
	if cfg.Global.Quiet == nil || !*cfg.Global.Quiet {
		t.Fatalf("expected global quiet=true, got %#v", cfg.Global.Quiet)
	}

	repo, ok := cfg.Repos["owner/repo"]
	if !ok {
		t.Fatalf("expected legacy repo section to load")
	}
	if repo.DownloadOnly == nil || !*repo.DownloadOnly {
		t.Fatalf("expected repo download_only=true, got %#v", repo.DownloadOnly)
	}
	if len(repo.AssetFilters) != 2 {
		t.Fatalf("expected repo asset filters to load, got %#v", repo.AssetFilters)
	}
	if repo.ExtractAll == nil || !*repo.ExtractAll {
		t.Fatalf("expected repo extract_all=true, got %#v", repo.ExtractAll)
	}
	if repo.IsGUI == nil || !*repo.IsGUI {
		t.Fatalf("expected repo is_gui=true, got %#v", repo.IsGUI)
	}
}

func TestLoadFileInitializesPackagesMapWhenSectionMissing(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	writeTestFile(t, configPath, `
[global]
target = "~/bin"
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load file: %v", err)
	}

	if cfg.Packages == nil {
		t.Fatal("expected packages map to be initialized")
	}
}

func TestLoadFileSupportsAPICacheAndGhproxySections(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	writeTestFile(t, configPath, `
[global]
target = "~/bin"

[api_cache]
enable = true
cache_time = 300

[ghproxy]
enable = true
host_url = "https://gh.felicity.ac.cn"
support_api = true
fallbacks = ["https://gh.llkk.cc", "https://gh.fhjhy.top"]
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load file: %v", err)
	}

	if cfg.ApiCache.Enable == nil || !*cfg.ApiCache.Enable {
		t.Fatalf("expected api_cache.enable=true, got %#v", cfg.ApiCache.Enable)
	}
	if cfg.ApiCache.CacheTime == nil || *cfg.ApiCache.CacheTime != 300 {
		t.Fatalf("expected api_cache.cache_time=300, got %#v", cfg.ApiCache.CacheTime)
	}
	if cfg.Ghproxy.Enable == nil || !*cfg.Ghproxy.Enable {
		t.Fatalf("expected ghproxy.enable=true, got %#v", cfg.Ghproxy.Enable)
	}
	if cfg.Ghproxy.HostURL == nil || *cfg.Ghproxy.HostURL != "https://gh.felicity.ac.cn" {
		t.Fatalf("expected ghproxy.host_url, got %#v", cfg.Ghproxy.HostURL)
	}
	if cfg.Ghproxy.SupportAPI == nil || !*cfg.Ghproxy.SupportAPI {
		t.Fatalf("expected ghproxy.support_api=true, got %#v", cfg.Ghproxy.SupportAPI)
	}
	if len(cfg.Ghproxy.Fallbacks) != 2 {
		t.Fatalf("expected ghproxy fallbacks to load, got %#v", cfg.Ghproxy.Fallbacks)
	}
	if _, ok := cfg.Repos["api_cache"]; ok {
		t.Fatalf("expected api_cache to not be treated as repo section")
	}
	if _, ok := cfg.Repos["ghproxy"]; ok {
		t.Fatalf("expected ghproxy to not be treated as repo section")
	}
}

func TestLoadFileDecodesConcurrencyOptions(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	writeTestFile(t, configPath, `
[global]
chunk_concurrency = 0
batch_concurrency = 3

[packages.fd]
repo = "sharkdp/fd"
chunk_concurrency = 2

["sharkdp/fd"]
chunk_concurrency = 4
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Global.ChunkConcurrency == nil || *cfg.Global.ChunkConcurrency != 0 {
		t.Fatalf("expected global chunk_concurrency=0, got %#v", cfg.Global.ChunkConcurrency)
	}
	if cfg.Global.BatchConcurrency == nil || *cfg.Global.BatchConcurrency != 3 {
		t.Fatalf("expected global batch_concurrency=3, got %#v", cfg.Global.BatchConcurrency)
	}
	if cfg.Packages["fd"].ChunkConcurrency == nil || *cfg.Packages["fd"].ChunkConcurrency != 2 {
		t.Fatalf("expected package chunk_concurrency=2, got %#v", cfg.Packages["fd"].ChunkConcurrency)
	}
	if cfg.Repos["sharkdp/fd"].ChunkConcurrency == nil || *cfg.Repos["sharkdp/fd"].ChunkConcurrency != 4 {
		t.Fatalf("expected repo chunk_concurrency=4, got %#v", cfg.Repos["sharkdp/fd"].ChunkConcurrency)
	}
}

func TestLoadFileReadsGlobalSys7zPath(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	writeTestFile(t, configPath, `
[global]
sys7z_path = "C:/Program Files/7-Zip/7z.exe"
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	assert.Eq(t, "C:/Program Files/7-Zip/7z.exe", *cfg.Global.Sys7zPath)
}

func TestLoadFileReadsGlobalIgnoreUpdatePackages(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	writeTestFile(t, configPath, `
[global]
ignore_update_packages = ["fzf", "rg"]
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	assert.Eq(t, []string{"fzf", "rg"}, cfg.Global.IgnoreUpdatePackages)
}

func TestLoadFileReadsSDKSections(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	writeTestFile(t, configPath, `
[global]
sdk_target = "~/sdks"
sdk_ext_map = { windows = "zip", linux = "tar.gz" }

[sdk.go]
aliases = ["golang"]
target = "gosdk/go{version}"
url_template = "https://example.com/go{version}.{os}-{arch}.{ext}"
index_url = "https://example.com/golang/"
index_format = "html"
index_parser = "go-json"
index_path_prefix = "/golang/"
filename_pattern = "go{version}.{os}-{arch}.{ext}"
strip_components = 1
os_map = { windows = "windows", linux = "linux" }
arch_map = { amd64 = "amd64", arm64 = "arm64" }
ext_map = { windows = "zip", linux = "tar.gz" }
`)

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Global.SDKTarget == nil || *cfg.Global.SDKTarget != "~/sdks" {
		t.Fatalf("expected global.sdk_target, got %#v", cfg.Global.SDKTarget)
	}
	assert.Eq(t, "tar.gz", cfg.Global.SDKExtMap["linux"])
	got := cfg.SDK["go"]
	assert.Eq(t, []string{"golang"}, got.Aliases)
	assert.Eq(t, "gosdk/go{version}", *got.Target)
	assert.Eq(t, "https://example.com/go{version}.{os}-{arch}.{ext}", *got.URLTemplate)
	assert.Eq(t, "https://example.com/golang/", *got.IndexURL)
	assert.Eq(t, "html", *got.IndexFormat)
	assert.Eq(t, "go-json", *got.IndexParser)
	assert.Eq(t, "/golang/", *got.IndexPathPrefix)
	assert.Eq(t, "go{version}.{os}-{arch}.{ext}", *got.FilenamePattern)
	assert.Eq(t, 1, *got.StripComponents)
	assert.Eq(t, "linux", got.OSMap["linux"])
	assert.Eq(t, "arm64", got.ArchMap["arm64"])
	assert.Eq(t, "zip", got.ExtMap["windows"])
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
