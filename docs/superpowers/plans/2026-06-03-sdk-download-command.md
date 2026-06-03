# SDK Download Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `eget sdk download|dl` to download SDK archives locally without extracting or recording installed SDKs.

**Architecture:** Reuse the existing SDK install resolution path by extracting shared archive-resolution logic from `Service.Install()`. Add a new SDK service method that downloads to the existing SDK archive cache, optionally copies the archive to an output directory, and returns a rich result for CLI rendering. Keep CLI changes aligned with existing `sdk install/list/search/index` routing and handler patterns.

**Tech Stack:** Go, `github.com/gookit/gcli/v3`, `github.com/gookit/goutil/testutil/assert`, existing `internal/sdk.DownloadArchive`, existing SDK index/config/template logic.

---

## Review Result

The approved design in `docs/superpowers/specs/2026-06-02-sdk-download-command-design.md` is implementable as-is. No design changes are required.

One implementation detail to keep explicit: both `internal/cli` and `internal/sdk` can define a type named `SDKDownloadOptions` because they live in different packages, but the service type should be referenced as `sdk.SDKDownloadOptions` in CLI code to keep the boundary clear.

Existing unrelated working-tree change:

```text
internal/cli/cache_cmd.go
```

Do not stage, modify, or revert that file while implementing this plan.

## File Structure

- Modify: `internal/sdk/service.go`
  - Add service-level download option/result types.
- Modify: `internal/sdk/config_resolve.go`
  - Add platform-aware config resolution helper so SDK download can resolve non-current platforms without mutating the service.
- Modify: `internal/sdk/install_service.go`
  - Extract shared archive resolution from `Install()`.
  - Keep install behavior unchanged.
- Create: `internal/sdk/download_service.go`
  - Implement `Service.Download()` and `Service.DownloadMany()`.
  - Implement output directory copy behavior.
- Create: `internal/sdk/download_service_test.go`
  - Cover exact URL template download, index-backed prefix/latest download, platform override, no installed-store record, output copy, and multi-target behavior.
- Modify: `internal/cli/service.go`
  - Add `DownloadMany` to `sdkCLIService`.
- Modify: `internal/cli/service_test.go`
  - Extend `fakeSDKService` for download handler tests.
- Modify: `internal/cli/sdk_cmd.go`
  - Add CLI `SDKDownloadOptions`, `newSDKDownloadCmd`, alias `dl`, flags `--os`, `--arch`, `--output/-o`.
- Modify: `internal/cli/handlers.go`
  - Route `sdk.download`.
- Modify: `internal/cli/sdk_handler.go`
  - Add `handleSDKDownload`.
- Modify: `internal/cli/app_sdk_test.go`
  - Add routing/binding tests for `sdk download` and `sdk dl`.
- Modify: `internal/cli/sdk_handler_test.go`
  - Add handler output and service-call tests.
- Modify: `docs/sdk-usage.md`
  - Document `sdk download/dl`.

## Task 1: SDK Service Download API Tests

**Files:**
- Create: `internal/sdk/download_service_test.go`
- Modify: `internal/sdk/service_test_helpers_test.go`

- [ ] **Step 1: Add SDK test config support for platform override**

In `internal/sdk/service_test_helpers_test.go`, update `testSDKConfig` so tests can resolve Windows archives and non-current platform files:

```go
func testSDKConfig(root string) *cfgpkg.File {
	cfg := cfgpkg.NewFile()
	cfg.Global.SDKTarget = stringPtr(filepath.Join(root, "sdks"))
	cfg.Global.SDKExtMap = map[string]string{"linux": "zip", "windows": "zip", "darwin": "tar.gz"}
	cfg.SDK["go"] = cfgpkg.SDKSection{
		Target:          stringPtr("gosdk/go{version}"),
		URLTemplate:     stringPtr("https://example.com/go{version}.{os}-{arch}.{ext}"),
		IndexURL:        stringPtr("https://example.com/golang/"),
		IndexFormat:     stringPtr("html"),
		FilenamePattern: stringPtr("go{version}.{os}-{arch}.{ext}"),
		StripComponents: intPtr(1),
	}
	return cfg
}
```

- [ ] **Step 2: Write failing exact-version download test**

Create `internal/sdk/download_service_test.go` with:

```go
package sdk

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestServiceDownloadExactVersionUsesURLTemplate(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.1", "go1.21.1.linux-amd64.zip")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://example.com/go1.21.1.linux-amd64.zip", req.URL)
			assert.Eq(t, "go1.21.1.linux-amd64.zip", req.Filename)
			assert.NotNil(t, req.Progress)
			return DownloadResult{Path: archivePath, Size: 7, FromCache: true}, nil
		},
	}

	result, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{
		Progress: func(size int64) io.Writer { return io.Discard },
	})

	assert.NoErr(t, err)
	assert.Eq(t, "go", result.Name)
	assert.Eq(t, "1.21.1", result.Version)
	assert.Eq(t, archivePath, result.Path)
	assert.Eq(t, "https://example.com/go1.21.1.linux-amd64.zip", result.URL)
	assert.Eq(t, "go1.21.1.linux-amd64.zip", result.Filename)
	assert.Eq(t, "linux", result.OS)
	assert.Eq(t, "amd64", result.Arch)
	assert.Eq(t, "zip", result.Ext)
	assert.True(t, result.Cached)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run:

```bash
go test ./internal/sdk -run TestServiceDownloadExactVersionUsesURLTemplate -count=1
```

Expected: FAIL because `Service.Download`, `SDKDownloadOptions`, and result types are not implemented.

- [ ] **Step 4: Add index/platform/output/multi tests**

Append these tests to `internal/sdk/download_service_test.go`:

```go
func TestServiceDownloadUsesIndexForPrefixVersion(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.13", "go1.21.13.linux-amd64.zip")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://mirror/go1.21.13.linux-amd64.zip", req.URL)
			return DownloadResult{Path: archivePath, Resumed: true}, nil
		},
	}
	assert.NoErr(t, svc.IndexCache.Save(Index{
		Schema:    1,
		SDK:       "go",
		SourceURL: "https://example.com/golang/",
		Items: []IndexItem{
			{Version: "1.21.1", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror/go1.21.1.linux-amd64.zip", Filename: "go1.21.1.linux-amd64.zip"}}},
			{Version: "1.21.13", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror/go1.21.13.linux-amd64.zip", Filename: "go1.21.13.linux-amd64.zip"}}},
		},
	}))

	result, err := svc.Download(context.Background(), "go:1.21", SDKDownloadOptions{})

	assert.NoErr(t, err)
	assert.Eq(t, "1.21.13", result.Version)
	assert.True(t, result.Resumed)
}

func TestServiceDownloadPlatformOverrideSelectsNonCurrentPlatform(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.1", "go1.21.1.windows-amd64.zip")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://example.com/go1.21.1.windows-amd64.zip", req.URL)
			return DownloadResult{Path: archivePath}, nil
		},
	}

	result, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{
		Platform: PlatformOptions{OS: "windows", Arch: "amd64"},
	})

	assert.NoErr(t, err)
	assert.Eq(t, "windows", result.OS)
	assert.Eq(t, "amd64", result.Arch)
	assert.Eq(t, "zip", result.Ext)
	assert.Eq(t, "go1.21.1.windows-amd64.zip", result.Filename)
}

func TestServiceDownloadRejectsPartialPlatformOverride(t *testing.T) {
	root := t.TempDir()
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}

	_, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{
		Platform: PlatformOptions{OS: "windows"},
	})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "--os and --arch")
}

func TestServiceDownloadDoesNotRecordInstalledSDK(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "archive.zip")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			return DownloadResult{Path: archivePath}, nil
		},
	}

	_, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{})

	assert.NoErr(t, err)
	_, statErr := os.Stat(svc.Store.Path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestServiceDownloadCopiesToOutputDir(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.1", "go1.21.1.linux-amd64.zip")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	outputDir := filepath.Join(root, "downloads")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			return DownloadResult{Path: archivePath, Size: 7}, nil
		},
	}

	result, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{OutputDir: outputDir})

	assert.NoErr(t, err)
	assert.Eq(t, filepath.Join(outputDir, "go1.21.1.linux-amd64.zip"), result.Path)
	data, err := os.ReadFile(result.Path)
	assert.NoErr(t, err)
	assert.Eq(t, "archive", string(data))
}

func TestServiceDownloadManyDownloadsTargetsInOrder(t *testing.T) {
	root := t.TempDir()
	var urls []string
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			urls = append(urls, req.URL)
			return DownloadResult{Path: filepath.Join(root, req.Filename)}, nil
		},
	}

	results, err := svc.DownloadMany(context.Background(), []string{"go@1.21.1", "go@1.22.0"}, SDKDownloadOptions{})

	assert.NoErr(t, err)
	assert.Eq(t, 2, len(results))
	assert.Eq(t, []string{
		"https://example.com/go1.21.1.linux-amd64.zip",
		"https://example.com/go1.22.0.linux-amd64.zip",
	}, urls)
}
```

- [ ] **Step 5: Run SDK download tests to verify they fail**

Run:

```bash
go test ./internal/sdk -run 'TestServiceDownload' -count=1
```

Expected: FAIL on missing service API and types.

- [ ] **Step 6: Commit failing tests**

```bash
git add internal/sdk/service_test_helpers_test.go internal/sdk/download_service_test.go
git commit -m "test(sdk): cover archive download service"
```

## Task 2: Shared SDK Archive Resolution

**Files:**
- Modify: `internal/sdk/service.go`
- Modify: `internal/sdk/config_resolve.go`
- Modify: `internal/sdk/install_service.go`

- [ ] **Step 1: Run GitNexus impact analysis before editing symbols**

Run:

```bash
npx gitnexus impact --repo eget --target Service.Install --direction upstream
npx gitnexus impact --repo eget --target Service.resolveConfig --direction upstream
```

Expected: The output reports direct callers and risk. If risk is HIGH or CRITICAL, stop and report before editing.

- [ ] **Step 2: Add service download types**

In `internal/sdk/service.go`, add these types near `InstallOptions`:

```go
type PlatformOptions struct {
	OS   string
	Arch string
}

type SDKDownloadOptions struct {
	Platform  PlatformOptions
	OutputDir string
	Progress  func(size int64) io.Writer
	OnStart   func(target string, version string, host string)
}

type SDKDownloadResult struct {
	Name     string
	Version  string
	Path     string
	URL      string
	Filename string
	OS       string
	Arch     string
	Ext      string
	Cached   bool
	Resumed  bool
}
```

- [ ] **Step 3: Add platform-aware config resolver**

In `internal/sdk/config_resolve.go`, replace `resolveConfig` with:

```go
func (s Service) resolveConfig(name string) (Config, error) {
	return s.resolveConfigForPlatform(name, PlatformOptions{})
}

func (s Service) resolveConfigForPlatform(name string, platform PlatformOptions) (Config, error) {
	goos := platform.OS
	if goos == "" {
		goos = s.GOOS
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := platform.Arch
	if goarch == "" {
		goarch = s.GOARCH
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return ResolveConfig(s.Config, name, ResolveConfigOptions{GOOS: goos, GOARCH: goarch})
}
```

- [ ] **Step 4: Add archive resolution structs and helper**

In `internal/sdk/install_service.go`, add this helper below `InstallMany` or near the top after imports:

```go
type resolvedArchive struct {
	RawTarget string
	Config    Config
	Version   string
	URL       string
	Filename  string
	OS        string
	Arch      string
	Ext       string
}

func (s Service) resolveDownloadArchive(rawTarget string, platform PlatformOptions) (resolvedArchive, error) {
	if (platform.OS == "") != (platform.Arch == "") {
		return resolvedArchive{}, fmt.Errorf("sdk download --os and --arch must be used together")
	}
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return resolvedArchive{}, err
	}
	cfg, err := s.resolveConfigForPlatform(target.Name, platform)
	if err != nil {
		return resolvedArchive{}, err
	}
	version, file, err := s.resolveVersionAndFile(target, cfg)
	if err != nil {
		return resolvedArchive{}, err
	}
	vars := TemplateVars{Name: cfg.Name, Version: version, OS: cfg.OS, Arch: cfg.Arch, Ext: cfg.Ext}
	url := file.URL
	filename := file.Filename
	if url == "" {
		url, err = RenderTemplate(cfg.URLTemplate, vars)
		if err != nil {
			return resolvedArchive{}, err
		}
		filename = filepath.Base(url)
	}
	if filename == "" {
		filename = filepath.Base(url)
	}
	return resolvedArchive{
		RawTarget: rawTarget,
		Config:    cfg,
		Version:   version,
		URL:       url,
		Filename:  filename,
		OS:        cfg.OS,
		Arch:      cfg.Arch,
		Ext:       cfg.Ext,
	}, nil
}
```

- [ ] **Step 5: Refactor `Install()` to use helper**

In `internal/sdk/install_service.go`, replace the initial target/config/version/url block inside `Install()` with:

```go
archive, err := s.resolveDownloadArchive(rawTarget, PlatformOptions{})
if err != nil {
	return InstallResult{}, err
}
cfg := archive.Config
vars := TemplateVars{Name: cfg.Name, Version: archive.Version, OS: archive.OS, Arch: archive.Arch, Ext: archive.Ext}
installPath, err := s.resolveInstallPath(cfg, vars)
if err != nil {
	return InstallResult{}, err
}
```

Then update remaining references in `Install()`:

```go
opts.OnStart(rawTarget, archive.Version, host)
```

```go
host := indexSourceHost(archive.URL)
```

```go
URL:      archive.URL,
SDK:      cfg.Name,
Version:  archive.Version,
Filename: archive.Filename,
```

```go
extractor := install.NewExtractor(archive.Filename, cfg.Name, chooser)
```

```go
tmpDir := filepath.Join(s.sdkRoot(cfg), ".eget-tmp", fmt.Sprintf("%s-%s-%d", cfg.Name, archive.Version, s.now().UnixNano()))
```

```go
entry := InstalledEntry{
	Name:            cfg.Name,
	Version:         archive.Version,
	Path:            installPath,
	URL:             archive.URL,
	Filename:        archive.Filename,
	OS:              archive.OS,
	Arch:            archive.Arch,
	Ext:             archive.Ext,
	InstalledAt:     s.now(),
	StripComponents: cfg.StripComponents,
}
```

```go
return InstallResult{Name: cfg.Name, Version: archive.Version, Path: installPath, URL: archive.URL, Cached: downloadResult.FromCache, Resumed: downloadResult.Resumed}, nil
```

- [ ] **Step 6: Run install tests to verify behavior is unchanged**

Run:

```bash
go test ./internal/sdk -run 'TestServiceInstall' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit shared resolution refactor**

```bash
git add internal/sdk/service.go internal/sdk/config_resolve.go internal/sdk/install_service.go
git commit -m "refactor(sdk): share archive resolution"
```

## Task 3: SDK Download Service Implementation

**Files:**
- Create: `internal/sdk/download_service.go`

- [ ] **Step 1: Run GitNexus impact analysis before adding related methods**

Run:

```bash
npx gitnexus impact --repo eget --target DownloadArchive --direction upstream
```

Expected: Existing install/download cache callers are listed. If risk is HIGH or CRITICAL, stop and report before editing.

- [ ] **Step 2: Implement download service**

Create `internal/sdk/download_service.go`:

```go
package sdk

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (s Service) Download(ctx context.Context, rawTarget string, opts SDKDownloadOptions) (SDKDownloadResult, error) {
	archive, err := s.resolveDownloadArchive(rawTarget, opts.Platform)
	if err != nil {
		return SDKDownloadResult{}, err
	}
	if opts.OnStart != nil {
		host := indexSourceHost(archive.URL)
		if host == "" {
			host = archive.URL
		}
		opts.OnStart(rawTarget, archive.Version, host)
	}
	download := s.Downloader
	if download == nil {
		download = DownloadArchive
	}
	downloadResult, err := download(ctx, DownloadRequest{
		URL:         archive.URL,
		CacheDir:    s.cacheDir(),
		SDK:         archive.Config.Name,
		Version:     archive.Version,
		Filename:    archive.Filename,
		ClientOpts:  s.effectiveClientOptions(),
		CacheMirror: s.CacheMirror,
		Progress:    opts.Progress,
	})
	if err != nil {
		return SDKDownloadResult{}, err
	}
	path := downloadResult.Path
	if opts.OutputDir != "" {
		path, err = copyDownloadedArchive(downloadResult.Path, opts.OutputDir, archive.Filename)
		if err != nil {
			return SDKDownloadResult{}, err
		}
	}
	return SDKDownloadResult{
		Name:     archive.Config.Name,
		Version:  archive.Version,
		Path:     path,
		URL:      archive.URL,
		Filename: archive.Filename,
		OS:       archive.OS,
		Arch:     archive.Arch,
		Ext:      archive.Ext,
		Cached:   downloadResult.FromCache,
		Resumed:  downloadResult.Resumed,
	}, nil
}

func (s Service) DownloadMany(ctx context.Context, targets []string, opts SDKDownloadOptions) ([]SDKDownloadResult, error) {
	results := make([]SDKDownloadResult, 0, len(targets))
	for _, target := range targets {
		result, err := s.Download(ctx, target, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func copyDownloadedArchive(src, outputDir, filename string) (string, error) {
	if info, err := os.Stat(outputDir); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("sdk download output path is not a directory: %s", outputDir)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	dst := filepath.Join(outputDir, filename)
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return dst, nil
}
```

- [ ] **Step 3: Run SDK download service tests**

Run:

```bash
go test ./internal/sdk -run 'TestServiceDownload' -count=1
```

Expected: PASS.

- [ ] **Step 4: Run full SDK package tests**

Run:

```bash
go test ./internal/sdk -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit download service implementation**

```bash
git add internal/sdk/download_service.go
git commit -m "feat(sdk): add archive download service"
```

## Task 4: CLI Command Routing and Handler

**Files:**
- Modify: `internal/cli/service.go`
- Modify: `internal/cli/service_test.go`
- Modify: `internal/cli/sdk_cmd.go`
- Modify: `internal/cli/handlers.go`
- Modify: `internal/cli/sdk_handler.go`
- Modify: `internal/cli/app_sdk_test.go`
- Modify: `internal/cli/sdk_handler_test.go`

- [ ] **Step 1: Run GitNexus impact analysis before CLI edits**

Run:

```bash
npx gitnexus impact --repo eget --target newSDKCmd --direction upstream
npx gitnexus impact --repo eget --target cliService.handle --direction upstream
```

Expected: CLI app construction and command routing are listed. If risk is HIGH or CRITICAL, stop and report before editing.

- [ ] **Step 2: Extend CLI service interface and fake service**

In `internal/cli/service.go`, add to `sdkCLIService`:

```go
DownloadMany(context.Context, []string, sdk.SDKDownloadOptions) ([]sdk.SDKDownloadResult, error)
```

In `internal/cli/service_test.go`, extend `fakeSDKService` fields:

```go
downloadTargets []string
downloadOpts    sdk.SDKDownloadOptions
downloadResults []sdk.SDKDownloadResult
```

Add method:

```go
func (f *fakeSDKService) DownloadMany(_ context.Context, targets []string, opts sdk.SDKDownloadOptions) ([]sdk.SDKDownloadResult, error) {
	f.downloadTargets = append([]string(nil), targets...)
	f.downloadOpts = opts
	for _, target := range targets {
		if opts.OnStart != nil {
			opts.OnStart(target, "1.21.1", "example.com")
		}
	}
	return f.downloadResults, f.err
}
```

- [ ] **Step 3: Add CLI routing tests**

In `internal/cli/app_sdk_test.go`, add cases to `TestMain_SDKRoutesAndBindsOptions`:

```go
{
	name:    "download default",
	args:    []string{"sdk", "download", "go:1.22"},
	wantCmd: "sdk.download",
	assertOpts: func(t *testing.T, options any) {
		opts, ok := options.(*SDKDownloadOptions)
		assert.True(t, ok)
		assert.Eq(t, []string{"go:1.22"}, opts.Targets)
		assert.Eq(t, "", opts.OS)
		assert.Eq(t, "", opts.Arch)
		assert.Eq(t, "", opts.Output)
	},
},
{
	name:    "download alias platform output",
	args:    []string{"sdk", "dl", "--os", "windows", "--arch", "amd64", "-o", "downloads", "go:1.22", "node:20"},
	wantCmd: "sdk.download",
	assertOpts: func(t *testing.T, options any) {
		opts, ok := options.(*SDKDownloadOptions)
		assert.True(t, ok)
		assert.Eq(t, []string{"go:1.22", "node:20"}, opts.Targets)
		assert.Eq(t, "windows", opts.OS)
		assert.Eq(t, "amd64", opts.Arch)
		assert.Eq(t, "downloads", opts.Output)
	},
},
```

Add a standalone validation test:

```go
func TestMain_SDKDownloadRejectsPartialPlatform(t *testing.T) {
	tests := [][]string{
		{"sdk", "download", "--os", "windows", "go:1.22"},
		{"sdk", "download", "--arch", "amd64", "go:1.22"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(func(string, any) error {
				t.Fatal("handler should not run")
				return nil
			}, &stdout, &stderr).RunWithArgs(args)
			assert.Err(t, err)
			assert.Contains(t, err.Error(), "--os and --arch")
		})
	}
}
```

- [ ] **Step 4: Add CLI options and command**

In `internal/cli/sdk_cmd.go`, add:

```go
type SDKDownloadOptions struct {
	Targets []string
	OS      string
	Arch    string
	Output  string
}
```

Inside `newSDKCmd`, create/reset options:

```go
downloadOpts := &SDKDownloadOptions{}
```

Add subcommand before install or after install:

```go
newSDKDownloadCmd(downloadOpts, handler),
```

Reset in returned cleanup:

```go
*downloadOpts = SDKDownloadOptions{}
```

Add function:

```go
func newSDKDownloadCmd(opts *SDKDownloadOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("download", "Download SDK archive(s)")
	cmd.Aliases = []string{"dl"}
	cmd.Config = func(c *gcli.Command) {
		c.StrOpt(&opts.OS, "os", "", "", "Target OS, default current OS")
		c.StrOpt(&opts.Arch, "arch", "", "", "Target arch, default current arch")
		c.StrOpt(&opts.Output, "output", "o", "", "Directory to place downloaded archive(s)")
		c.AddArg("target", "SDK target(s), for example go@1.22.0 or node:20.11.1", true, true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		targetArgs := append(c.Arg("target").Strings(), args...)
		if err := validateNoFlagArgs(targetArgs); err != nil {
			return err
		}
		if (opts.OS == "") != (opts.Arch == "") {
			return fmt.Errorf("sdk download --os and --arch must be used together")
		}
		opts.Targets = splitTargets(targetArgs)
		snapshot := *opts
		snapshot.Targets = append([]string(nil), opts.Targets...)
		return handler("sdk.download", &snapshot)
	}
	return cmd
}
```

Update `cmd.Help` examples:

```text
  eget sdk download go:1.22
  eget sdk dl --os linux --arch arm64 -o ./downloads go:1.22
```

- [ ] **Step 5: Route handler name**

In `internal/cli/handlers.go`, add before `sdk.install`:

```go
case "sdk.download":
	opts := options.(*SDKDownloadOptions)
	return s.handleSDKDownload(opts)
```

- [ ] **Step 6: Add handler tests**

In `internal/cli/sdk_handler_test.go`, add:

```go
func TestHandleSDKDownloadPrintsResults(t *testing.T) {
	fake := &fakeSDKService{
		downloadResults: []sdk.SDKDownloadResult{{
			Name: "go", Version: "1.21.1", Path: "/cache/go.zip", OS: "windows", Arch: "amd64", Cached: true, Resumed: true,
		}},
	}
	svc := &cliService{sdkService: fake}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.download", &SDKDownloadOptions{Targets: []string{"go@1.21.1"}, OS: "windows", Arch: "amd64", Output: "downloads"})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"go@1.21.1"}, fake.downloadTargets)
	assert.Eq(t, "windows", fake.downloadOpts.Platform.OS)
	assert.Eq(t, "amd64", fake.downloadOpts.Platform.Arch)
	assert.Eq(t, "downloads", fake.downloadOpts.OutputDir)
	assert.NotNil(t, fake.downloadOpts.Progress)
	got := out.String()
	assert.Contains(t, got, "Download SDK go@1.21.1 -> 1.21.1 from example.com")
	assert.Contains(t, got, "go@1.21.1")
	assert.Contains(t, got, "windows/amd64")
	assert.Contains(t, got, "/cache/go.zip")
	assert.Contains(t, got, "cached")
	assert.Contains(t, got, "resumed")
}

func TestHandleSDKDownloadRejectsMissingTarget(t *testing.T) {
	svc := &cliService{sdkService: &fakeSDKService{}}

	err := svc.handle("sdk.download", &SDKDownloadOptions{})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "target is required")
}
```

- [ ] **Step 7: Implement CLI handler**

In `internal/cli/sdk_handler.go`, add:

```go
func (s *cliService) handleSDKDownload(opts *SDKDownloadOptions) error {
	if opts == nil || len(opts.Targets) == 0 {
		return fmt.Errorf("sdk download target is required")
	}
	results, err := s.sdkService.DownloadMany(context.Background(), opts.Targets, sdk.SDKDownloadOptions{
		Platform:  sdk.PlatformOptions{OS: opts.OS, Arch: opts.Arch},
		OutputDir: opts.Output,
		Progress:  s.sdkDownloadProgress(),
		OnStart: func(target string, version string, host string) {
			ccolor.Printf(" - Download SDK %s -> %s from %s\n", target, version, host)
		},
	})
	if err != nil {
		return err
	}
	for _, result := range results {
		notes := clirender.SDKResultNotes(result.Cached, result.Resumed)
		platform := result.OS + "/" + result.Arch
		if notes != "" {
			ccolor.Successf("✓ Downloaded %s@%s %s -> %s (%s)\n", result.Name, result.Version, platform, result.Path, notes)
			continue
		}
		ccolor.Successf("✓ Downloaded %s@%s %s -> %s\n", result.Name, result.Version, platform, result.Path)
	}
	return nil
}
```

- [ ] **Step 8: Run CLI tests**

Run:

```bash
go test ./internal/cli -run 'TestMain_SDK|TestHandleSDKDownload' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit CLI integration**

```bash
git add internal/cli/service.go internal/cli/service_test.go internal/cli/sdk_cmd.go internal/cli/handlers.go internal/cli/sdk_handler.go internal/cli/app_sdk_test.go internal/cli/sdk_handler_test.go
git commit -m "feat(cli): add sdk download command"
```

## Task 5: Documentation and Final Verification

**Files:**
- Modify: `docs/sdk-usage.md`
- Modify: `docs/superpowers/plans/2026-06-03-sdk-download-command.md`

- [ ] **Step 1: Update SDK usage command overview**

In `docs/sdk-usage.md`, add examples near the command overview:

```bash
eget sdk download go:1.22
eget sdk dl --os linux --arch arm64 go:1.22
eget sdk dl -o ./downloads go:1.22
```

Add alias bullet:

```text
- `sdk download`: `sdk dl`
```

- [ ] **Step 2: Add SDK download usage section**

In `docs/sdk-usage.md`, add a section after "安装和删除":

````markdown
## 只下载归档

`sdk download` 只下载 SDK 归档，不解压、不写安装记录：

```bash
eget sdk download go:1.22
eget sdk dl go:1.22
```

默认下载当前平台归档，文件保存在 `{cache_dir}/sdk-downloads/{sdk}/{version}/`。如果归档已经完整存在且 meta 匹配，会直接复用缓存。

可以下载非当前平台归档：

```bash
eget sdk dl --os linux --arch arm64 go:1.22
eget sdk dl --os windows --arch amd64 node:20
```

`--os` 和 `--arch` 使用 Go 平台名，例如 `linux`、`windows`、`darwin`、`amd64`、`arm64`。发布文件名里的平台命名仍由 SDK 配置中的 `os_map`、`arch_map` 和 `ext_map` 决定。

可以把最终归档复制到指定目录：

```bash
eget sdk dl -o ./downloads go:1.22
```

`--output/-o` 只表示目录，不支持改名为单个文件。
````

- [ ] **Step 3: Run focused package tests**

Run:

```bash
go test ./internal/sdk ./internal/cli -count=1
```

Expected: PASS.

- [ ] **Step 4: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Run GitNexus detect changes before final commit**

Run:

```bash
npx gitnexus detect_changes --repo eget
```

Expected: Reported changes should be limited to SDK service, SDK CLI, tests, and docs. If unrelated symbols or high-risk flows appear, review before committing.

- [ ] **Step 6: Mark this implementation plan checkboxes as complete**

Update this plan file as each task is completed. At this final step, every executed checkbox should be marked `[x]`.

- [ ] **Step 7: Commit docs and plan progress**

```bash
git add docs/sdk-usage.md docs/superpowers/plans/2026-06-03-sdk-download-command.md
git commit -m "docs: document sdk download command"
```

## Completion Criteria

- `eget sdk download go:1.22` routes to `sdk.download`.
- `eget sdk dl go:1.22` works as an alias.
- `--os` and `--arch` together select a non-current platform using existing SDK config maps.
- Passing only `--os` or only `--arch` returns a clear error.
- Default downloads use `{cache_dir}/sdk-downloads/{sdk}/{version}/{filename}`.
- `--output/-o` copies the archive to the requested directory.
- Download does not extract archives.
- Download does not write `sdk.installed.json`.
- Existing `sdk install` tests still pass.
- `go test ./...` passes.
- `npx gitnexus detect_changes --repo eget` has been run before final commit.
