package app

import (
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/util"
)

func TestInstallTargetUsesConfiguredDefaults(t *testing.T) {
	cfg := mustLoadFromString(t, `
[global]
target = "~/.local/bin"
cache_dir = "~/.cache/eget"
proxy_url = "http://127.0.0.1:7890"
`)
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://example.com/tool.tar.gz",
			ExtractedFiles: []string{"./tool"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("junegunn/fzf", install.Options{})
	if err != nil {
		t.Fatalf("install target: %v", err)
	}

	expectedTarget, err := util.Expand("~/.local/bin")
	if err != nil {
		t.Fatalf("expand target: %v", err)
	}
	expectedCache, err := util.Expand("~/.cache/eget")
	if err != nil {
		t.Fatalf("expand cache: %v", err)
	}

	if runner.opts.Output != expectedTarget {
		t.Fatalf("expected configured install target, got %q", runner.opts.Output)
	}
	if runner.opts.CacheDir != expectedCache {
		t.Fatalf("expected configured cache dir, got %q", runner.opts.CacheDir)
	}
	if runner.opts.ProxyURL != "http://127.0.0.1:7890" {
		t.Fatalf("expected configured proxy url, got %q", runner.opts.ProxyURL)
	}
	if expected := filepath.Join(expectedCache, "api-cache"); runner.opts.APICacheDir != expected {
		t.Fatalf("expected derived api cache dir, got %q", runner.opts.APICacheDir)
	}
}

func TestInstallTargetUsesDefaultTargetWhenGlobalTargetMissingOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		cfg  *cfgpkg.File
	}{
		{
			name: "missing",
			cfg:  cfgpkg.NewFile(),
		},
		{
			name: "empty",
			cfg: func() *cfgpkg.File {
				cfg := cfgpkg.NewFile()
				cfg.Global.Target = util.StringPtr("")
				return cfg
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{}
			svc := Service{
				Runner: runner,
				LoadConfig: func() (*cfgpkg.File, error) {
					return tt.cfg, nil
				},
			}

			_, err := svc.InstallTarget("junegunn/fzf", install.Options{})

			assert.NoErr(t, err)
			expectedTarget, err := util.Expand("~/.local/bin")
			assert.NoErr(t, err)
			assert.Eq(t, expectedTarget, runner.opts.Output)
			assert.False(t, runner.opts.OutputExplicit)
		})
	}
}

func TestInstallTargetResolvesManagedPackageName(t *testing.T) {
	cfg := mustLoadFromString(t, `
[global]
target = "~/.local/bin"

["sipeed/picoclaw"]
system = "windows/amd64"

[packages.picoclaw]
repo = "sipeed/picoclaw"
desc = "Manual PicoClaw description"
target = "D:/Program/AITools/PicoClaw"
tag = "v1.2.3"
file = "*.exe"
asset_filters = ["windows"]
`)
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://github.com/sipeed/picoclaw/releases/download/v1.2.3/picoclaw.zip",
			ExtractedFiles: []string{"./picoclaw.exe"},
		},
	}
	store := &fakeInstalledStore{}
	svc := Service{
		Runner: runner,
		Store:  store,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
		RepoMetadata: func(repo string) (RepoMetadata, error) {
			return RepoMetadata{Desc: "Repository PicoClaw description"}, nil
		},
	}

	_, err := svc.InstallTarget("picoclaw", install.Options{})
	if err != nil {
		t.Fatalf("install target: %v", err)
	}

	if runner.target != "sipeed/picoclaw" {
		t.Fatalf("expected managed package to resolve repo, got %q", runner.target)
	}
	if runner.opts.Output != "D:/Program/AITools/PicoClaw" {
		t.Fatalf("expected package target to be used, got %q", runner.opts.Output)
	}
	if runner.opts.System != "windows/amd64" {
		t.Fatalf("expected repo system to be merged, got %q", runner.opts.System)
	}
	if runner.opts.Tag != "v1.2.3" {
		t.Fatalf("expected package tag to be merged, got %q", runner.opts.Tag)
	}
	if runner.opts.ExtractFile != "*.exe" {
		t.Fatalf("expected package file glob to be merged, got %q", runner.opts.ExtractFile)
	}
	if !runner.opts.All {
		t.Fatal("expected file glob to enable extract-all mode")
	}
	if len(runner.opts.Asset) != 1 || runner.opts.Asset[0] != "windows" {
		t.Fatalf("expected package asset filter to be merged, got %#v", runner.opts.Asset)
	}
	if store.target != "picoclaw" {
		t.Fatalf("expected installed store to record package name, got %q", store.target)
	}
	if store.entry.Repo != "sipeed/picoclaw" {
		t.Fatalf("expected installed repo sipeed/picoclaw, got %q", store.entry.Repo)
	}
	if store.entry.Target != "sipeed/picoclaw" {
		t.Fatalf("expected installed target to be real repo, got %q", store.entry.Target)
	}
	assert.Eq(t, "Manual PicoClaw description", store.entry.Desc)
}

func TestInstallTargetMergesTemplatePackageOptions(t *testing.T) {
	cfg := mustLoadFromString(t, `
[packages.claude]
repo = "template:claude"
latest_url = "https://example.com/latest"
latest_format = "text"
url_template = "https://example.com/{version}/{os}-{arch}/claude{ext}"
os_map = { windows = "win32" }
arch_map = { amd64 = "x64" }
ext_map = { windows = ".exe" }
install_action = "run-asset"
install_args = ["install", "latest"]
`)
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://example.com/1.2.3/win32-x64/claude.exe",
			Tool:           "claude",
			ExtractedFiles: []string{"./claude.exe"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("claude", install.Options{})
	if err != nil {
		t.Fatalf("install template package: %v", err)
	}

	assert.Eq(t, "template:claude", runner.target)
	assert.Eq(t, "https://example.com/latest", runner.opts.URLTemplate.LatestURL)
	assert.Eq(t, "https://example.com/{version}/{os}-{arch}/claude{ext}", runner.opts.URLTemplate.URLTemplate)
	assert.Eq(t, map[string]string{"windows": "win32"}, runner.opts.URLTemplate.OSMap)
	assert.Eq(t, map[string]string{"amd64": "x64"}, runner.opts.URLTemplate.ArchMap)
	assert.Eq(t, map[string]string{"windows": ".exe"}, runner.opts.URLTemplate.ExtMap)
	assert.Eq(t, "run-asset", runner.opts.URLTemplate.InstallAction)
	assert.Eq(t, []string{"install", "latest"}, runner.opts.URLTemplate.InstallArgs)
}

func TestInstallTargetAppliesManagedPackageOptionsWhenTargetIsRepo(t *testing.T) {
	cfg := mustLoadFromString(t, `
[packages.erd]
repo = "solidiquis/erdtree"
name = "erd"
file = "erd"
asset_filters = ["musl"]
rename_files = { "erdtree" = "erd" }
strip_components = 1
`)
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://github.com/solidiquis/erdtree/releases/download/v3.1.2/erdtree.tar.gz",
			ExtractedFiles: []string{"./erd"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("solidiquis/erdtree", install.Options{})
	if err != nil {
		t.Fatalf("install target: %v", err)
	}

	if runner.target != "solidiquis/erdtree" {
		t.Fatalf("expected repo target to be installed, got %q", runner.target)
	}
	if runner.opts.Name != "erd" {
		t.Fatalf("expected package name to be merged, got %q", runner.opts.Name)
	}
	if runner.opts.ExtractFile != "erd" {
		t.Fatalf("expected package file to be merged, got %q", runner.opts.ExtractFile)
	}
	if len(runner.opts.Asset) != 1 || runner.opts.Asset[0] != "musl" {
		t.Fatalf("expected package asset filter to be merged, got %#v", runner.opts.Asset)
	}
	assert.Eq(t, map[string]string{"erdtree": "erd"}, runner.opts.RenameFiles)
	assert.Eq(t, 1, runner.opts.StripComponents)
}

func TestInstallTargetAllowsCLINameToOverrideManagedPackageName(t *testing.T) {
	cfg := mustLoadFromString(t, `
[packages.erd]
repo = "solidiquis/erdtree"
name = "erd"
file = "erd"
`)
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://github.com/solidiquis/erdtree/releases/download/v3.1.2/erdtree.tar.gz",
			ExtractedFiles: []string{"./erd"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("solidiquis/erdtree", install.Options{Name: "custom-erd"})
	if err != nil {
		t.Fatalf("install target: %v", err)
	}

	if runner.opts.Name != "custom-erd" {
		t.Fatalf("expected CLI name to override package name, got %q", runner.opts.Name)
	}
}

func TestInstallTargetRejectsManagedPackageWithoutRepo(t *testing.T) {
	cfg := mustLoadFromString(t, `
[packages.picoclaw]
target = "D:/Program/AITools/PicoClaw"
`)
	svc := Service{
		Runner: &fakeRunner{},
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("picoclaw", install.Options{})
	if err == nil {
		t.Fatal("expected install target to fail when package repo is missing")
	}
	if err.Error() != `package "picoclaw" has no repo` {
		t.Fatalf("unexpected error: %v", err)
	}
}
