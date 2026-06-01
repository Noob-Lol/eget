package app

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/util"
)

func TestDownloadTargetRunsWithoutRecordingInstalledState(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://go.dev/dl/go1.22.0.linux-amd64.tar.gz",
			ExtractedFiles: []string{"./go1.22.0.tar.gz"},
		},
	}
	store := &fakeInstalledStore{}
	svc := Service{Runner: runner, Store: store}

	opts := install.Options{System: "linux/amd64"}
	result, err := svc.DownloadTarget("https://go.dev/dl/go1.22.0.linux-amd64.tar.gz", opts)
	if err != nil {
		t.Fatalf("download target: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("expected runner to be called once, got %d", runner.calls)
	}
	if !runner.opts.DownloadOnly {
		t.Fatalf("expected download target to force DownloadOnly=true")
	}
	if store.calls != 0 {
		t.Fatalf("expected store to not be called, got %d", store.calls)
	}
	if result.URL == "" {
		t.Fatal("expected result URL to be preserved")
	}
}

func TestDownloadTargetDoesNotUseDefaultInstallTarget(t *testing.T) {
	runner := &fakeRunner{}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
	}

	_, err := svc.DownloadTarget("https://example.com/tool.tar.gz", install.Options{})

	assert.NoErr(t, err)
	assert.Eq(t, "", runner.opts.Output)
}

func TestDownloadTargetForwardsFallbackVersions(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.59/Ukrainian.zip",
			ExtractedFiles: []string{"./Ukrainian.zip"},
		},
	}
	svc := Service{Runner: runner}

	_, err := svc.DownloadTarget("sourceforge:keepass/Translations 2.x", install.Options{
		FallbackVersions: 10,
		Asset:            []string{"Ukrainian", "zip"},
	})

	if err != nil {
		t.Fatalf("download target: %v", err)
	}
	if runner.opts.FallbackVersions != 10 {
		t.Fatalf("expected fallback versions to be forwarded, got %d", runner.opts.FallbackVersions)
	}
}

func TestDownloadTargetWithExtractFileRunsExtractionFlow(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://example.com/tool.tar.gz",
			ExtractedFiles: []string{"./tool"},
		},
	}
	store := &fakeInstalledStore{}
	svc := Service{Runner: runner, Store: store}

	_, err := svc.DownloadTarget("https://example.com/tool.tar.gz", install.Options{ExtractFile: "tool"})
	if err != nil {
		t.Fatalf("download target: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("expected runner to be called once, got %d", runner.calls)
	}
	if runner.opts.DownloadOnly {
		t.Fatal("expected download target with --file to disable DownloadOnly")
	}
	if store.calls != 0 {
		t.Fatalf("expected store to not be called, got %d", store.calls)
	}
}

func TestDownloadTargetWithGlobExtractFileEnablesExtractAll(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://example.com/tool.zip",
			ExtractedFiles: []string{"./picoclaw.exe", "./picoclaw-launcher.exe"},
		},
	}
	svc := Service{Runner: runner}

	_, err := svc.DownloadTarget("https://example.com/tool.zip", install.Options{ExtractFile: "*.exe"})
	if err != nil {
		t.Fatalf("download target: %v", err)
	}

	if !runner.opts.All {
		t.Fatal("expected glob extract file to enable extract-all mode")
	}
	if runner.opts.DownloadOnly {
		t.Fatal("expected glob extract file to disable DownloadOnly")
	}
}

func TestDownloadTargetWithAllRunsExtractionFlow(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://example.com/tool.zip",
			ExtractedFiles: []string{"./bin/tool.exe"},
		},
	}
	store := &fakeInstalledStore{}
	svc := Service{Runner: runner, Store: store}

	_, err := svc.DownloadTarget("https://example.com/tool.zip", install.Options{All: true})
	if err != nil {
		t.Fatalf("download target: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("expected runner to be called once, got %d", runner.calls)
	}
	if runner.opts.DownloadOnly {
		t.Fatal("expected download target with extract-all to disable DownloadOnly")
	}
	if store.calls != 0 {
		t.Fatalf("expected store to not be called, got %d", store.calls)
	}
}

func TestDownloadTargetAcceptsManagedTemplatePackageName(t *testing.T) {
	cfg := mustLoadFromString(t, `
[packages.claude]
repo = "template:claude"
latest_url = "https://example.com/latest"
latest_format = "text"
url_template = "https://example.com/{version}/{os}-{arch}/claude{ext}"
os_map = { windows = "win32" }
arch_map = { amd64 = "x64" }
ext_map = { windows = ".exe" }
`)
	runner := &fakeRunner{
		result: RunResult{
			URL:   "https://example.com/1.2.3/win32-x64/claude.exe",
			Tool:  "claude",
			Asset: "claude.exe",
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	_, err := svc.DownloadTarget("claude", install.Options{})
	if err != nil {
		t.Fatalf("download template package: %v", err)
	}

	assert.Eq(t, "template:claude", runner.target)
	assert.True(t, runner.opts.DownloadOnly)
	assert.Eq(t, "https://example.com/latest", runner.opts.URLTemplate.LatestURL)
	assert.Eq(t, "https://example.com/{version}/{os}-{arch}/claude{ext}", runner.opts.URLTemplate.URLTemplate)
	assert.Eq(t, map[string]string{"windows": "win32"}, runner.opts.URLTemplate.OSMap)
	assert.Eq(t, map[string]string{"amd64": "x64"}, runner.opts.URLTemplate.ArchMap)
	assert.Eq(t, map[string]string{"windows": ".exe"}, runner.opts.URLTemplate.ExtMap)
}

func TestDownloadTargetUsesConfiguredCacheDirByDefault(t *testing.T) {
	cfg := mustLoadFromString(t, `
[global]
target = "~/.local/bin"
cache_dir = "~/.cache/eget"
`)
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://example.com/tool.tar.gz",
			ExtractedFiles: []string{"./tool.tar.gz"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	_, err := svc.DownloadTarget("https://example.com/tool.tar.gz", install.Options{})
	if err != nil {
		t.Fatalf("download target: %v", err)
	}

	expectedCache, err := util.Expand("~/.cache/eget")
	if err != nil {
		t.Fatalf("expand cache: %v", err)
	}

	if runner.opts.Output != expectedCache {
		t.Fatalf("expected configured cache dir as download output, got %q", runner.opts.Output)
	}
	if runner.opts.CacheDir != expectedCache {
		t.Fatalf("expected configured cache dir, got %q", runner.opts.CacheDir)
	}
}

func TestDownloadTargetPreservesExplicitCacheMeta(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://example.com/tools/eget/eget-windows-amd64.exe",
			ExtractedFiles: []string{"./eget.exe"},
		},
	}
	svc := Service{Runner: runner}

	_, err := svc.DownloadTarget("https://example.com/tools/eget/eget-windows-amd64.exe", install.Options{
		CacheName:    "eget",
		CacheVersion: "1.7.2-45-g5286225",
	})
	if err != nil {
		t.Fatalf("download target: %v", err)
	}

	assert.Eq(t, "eget", runner.opts.CacheName)
	assert.Eq(t, "1.7.2-45-g5286225", runner.opts.CacheVersion)
}
