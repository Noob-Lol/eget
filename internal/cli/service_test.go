package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gookit/cliui"
	"github.com/gookit/goutil/testutil/assert"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/app"
	"github.com/inherelab/eget/internal/client"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	storepkg "github.com/inherelab/eget/internal/installed"
	"github.com/inherelab/eget/internal/sdk"
	forge "github.com/inherelab/eget/internal/source/forge"
	sourcegithub "github.com/inherelab/eget/internal/source/github"
	sourcesf "github.com/inherelab/eget/internal/source/sourceforge"
	"github.com/inherelab/eget/internal/util"
)

type fakeSDKService struct {
	installTargets []string
	installOpts    sdk.InstallOptions
	installResults []sdk.InstallResult
	listName       string
	listEntries    []sdk.InstalledEntry
	removeTarget   string
	removeResult   sdk.RemoveResult
	indexName      string
	indexAll       bool
	index          sdk.Index
	indexes        []sdk.Index
	cachedIndexes  []sdk.CachedIndexInfo
	searchName     string
	searchKeywords []string
	searchNumber   int
	searchSort     string
	searchResults  []sdk.SearchResult
	clearName      string
	clearAll       bool
	err            error
}

type fakeUninstallStoreForCLI struct {
	cfg        *storepkg.Config
	removeKeys []string
}

func writeCLIFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func (f *fakeUninstallStoreForCLI) Load() (*storepkg.Config, error) {
	return f.cfg, nil
}

func (f *fakeUninstallStoreForCLI) Remove(target string) error {
	f.removeKeys = append(f.removeKeys, target)
	return nil
}

func (f *fakeSDKService) InstallMany(_ context.Context, targets []string, opts sdk.InstallOptions) ([]sdk.InstallResult, error) {
	f.installTargets = append([]string(nil), targets...)
	f.installOpts = opts
	for _, target := range targets {
		if opts.OnStart != nil {
			opts.OnStart(target, "1.21.1", "example.com")
		}
	}
	return f.installResults, f.err
}

func (f *fakeSDKService) List(name string) ([]sdk.InstalledEntry, error) {
	f.listName = name
	return f.listEntries, f.err
}

func (f *fakeSDKService) Remove(target string) (sdk.RemoveResult, error) {
	f.removeTarget = target
	return f.removeResult, f.err
}

func (f *fakeSDKService) RefreshIndex(_ context.Context, name string) (sdk.Index, error) {
	f.indexName = name
	return f.index, f.err
}

func (f *fakeSDKService) RefreshAllIndexes(_ context.Context) ([]sdk.Index, error) {
	f.indexAll = true
	return f.indexes, f.err
}

func (f *fakeSDKService) ShowIndex(name string) (sdk.Index, error) {
	f.indexName = name
	return f.index, f.err
}

func (f *fakeSDKService) ListIndexes() ([]sdk.CachedIndexInfo, error) {
	return f.cachedIndexes, f.err
}

func (f *fakeSDKService) SearchIndex(name string, opts sdk.SearchOptions) ([]sdk.SearchResult, error) {
	f.searchName = name
	f.searchKeywords = append([]string(nil), opts.Keywords...)
	f.searchNumber = opts.Number
	f.searchSort = opts.Sort
	return f.searchResults, f.err
}

func (f *fakeSDKService) ClearIndex(name string) error {
	f.clearName = name
	return f.err
}

func (f *fakeSDKService) ClearAllIndexes() error {
	f.clearAll = true
	return f.err
}

func TestInstallOptionsFromCommandsDoNotSetCacheDir(t *testing.T) {
	installOpts := installOptionsFromInstall(&InstallOptions{
		Tag:             "nightly",
		System:          "linux/amd64",
		To:              "~/.local/bin",
		File:            "tool",
		Asset:           "linux",
		Rename:          "tool-linux-amd64=tool",
		Source:          true,
		All:             true,
		Quiet:           true,
		StripComponents: 1,
		Add:             true,
		Name:            "tool",
	})
	if installOpts.CacheDir != "" {
		t.Fatalf("expected install cache dir to stay empty, got %q", installOpts.CacheDir)
	}
	if installOpts.Name != "tool" {
		t.Fatalf("expected install name to propagate, got %q", installOpts.Name)
	}
	assert.Eq(t, map[string]string{"tool-linux-amd64": "tool"}, installOpts.RenameFiles)
	assert.Eq(t, 1, installOpts.StripComponents)

	downloadOpts := installOptionsFromDownload(&DownloadOptions{
		Tag:             "nightly",
		System:          "linux/amd64",
		To:              "~/.cache/downloads",
		Asset:           "linux",
		Rename:          "tool-linux-amd64=tool",
		Source:          true,
		Quiet:           true,
		StripComponents: 1,
	})
	if downloadOpts.CacheDir != "" {
		t.Fatalf("expected download cache dir to stay empty, got %q", downloadOpts.CacheDir)
	}
	if !downloadOpts.DownloadOnly {
		t.Fatal("expected plain download options to default to raw download mode")
	}
	assert.Eq(t, 1, downloadOpts.StripComponents)

	addOpts := installOptionsFromAdd(&AddOptions{
		Name:            "tool",
		Tag:             "nightly",
		System:          "linux/amd64",
		To:              "~/.local/bin",
		File:            "tool",
		Asset:           "linux",
		Rename:          "tool-linux-amd64=tool",
		Source:          true,
		All:             true,
		Quiet:           true,
		StripComponents: 1,
	})
	if addOpts.CacheDir != "" {
		t.Fatalf("expected add cache dir to stay empty, got %q", addOpts.CacheDir)
	}
	assert.Eq(t, map[string]string{"tool-linux-amd64": "tool"}, addOpts.RenameFiles)
	assert.Eq(t, 1, addOpts.StripComponents)

	updateOpts := installOptionsFromUpdate(&UpdateOptions{
		Tag:    "nightly",
		System: "linux/amd64",
		To:     "~/.local/bin",
		Asset:  "linux",
		Source: true,
		Quiet:  true,
	})
	if updateOpts.CacheDir != "" {
		t.Fatalf("expected update cache dir to stay empty, got %q", updateOpts.CacheDir)
	}
}

func TestApplyGlobalNetworkConfigDerivesAPICacheDir(t *testing.T) {
	cacheDir := "~/.cache/eget"
	proxyURL := "http://127.0.0.1:7890"
	apiCacheEnabled := true
	apiCacheTime := 120
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &cacheDir
	cfg.Global.ProxyURL = &proxyURL
	cfg.ApiCache.Enable = &apiCacheEnabled
	cfg.ApiCache.CacheTime = &apiCacheTime

	opts := install.Options{}
	applyGlobalNetworkConfig(&opts, cfg)

	expectedCache, err := util.Expand(cacheDir)
	assert.NoErr(t, err)
	assert.True(t, opts.APICacheEnabled)
	assert.Eq(t, 120, opts.APICacheTime)
	assert.Eq(t, filepath.Join(expectedCache, "api-cache"), opts.APICacheDir)
	assert.Eq(t, proxyURL, opts.ProxyURL)
}

func TestInstallOptionsFromDownloadEnablesArchiveExtractionWhenRequested(t *testing.T) {
	opts := installOptionsFromDownload(&DownloadOptions{
		File: "tool,LICENSE",
	})
	if opts.DownloadOnly {
		t.Fatal("expected download with --file to disable DownloadOnly")
	}
	if opts.ExtractFile != "tool,LICENSE" {
		t.Fatalf("expected extract file filters to propagate, got %q", opts.ExtractFile)
	}

	opts = installOptionsFromDownload(&DownloadOptions{
		All: true,
	})
	if opts.DownloadOnly {
		t.Fatal("expected download with extract-all to disable DownloadOnly")
	}
}

func TestPromptIndexConsumesTrailingNewline(t *testing.T) {
	origStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = origStdin
		_ = reader.Close()
	}()
	if _, err := writer.WriteString("14\ny\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}

	choices := make([]string, 14)
	for i := range choices {
		choices[i] = "choice"
	}
	picked, err := promptIndex(choices)
	if err != nil {
		t.Fatalf("prompt index: %v", err)
	}
	if picked != 13 {
		t.Fatalf("expected zero-based selection 13, got %d", picked)
	}

	rest, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatalf("read remaining stdin: %v", err)
	}
	if string(rest) != "y\n" {
		t.Fatalf("expected prompt index to consume selection newline, remaining stdin %q", rest)
	}
}

func TestPromptIndexRendersInteractiveSelect(t *testing.T) {
	origStdin := os.Stdin
	origStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	errReader, errWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stdin = reader
	os.Stderr = errWriter
	defer func() {
		os.Stdin = origStdin
		os.Stderr = origStderr
		_ = reader.Close()
		_ = errReader.Close()
		_ = errWriter.Close()
	}()
	if _, err := writer.WriteString("2\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}

	picked, err := promptIndex([]string{"first.zip", "second.zip"})
	assert.NoErr(t, err)
	assert.Eq(t, 1, picked)

	if err := errWriter.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	rendered, err := io.ReadAll(errReader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	got := string(rendered)
	assert.Contains(t, got, "Select package resource (2)")
	assert.Contains(t, got, "1) first.zip")
	assert.Contains(t, got, "2) second.zip")
}

func TestHandleInstallPrintsAddedPackageMessage(t *testing.T) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	svc := &cliService{
		appService: app.Service{
			Runner: &fakeRunnerForCLI{
				result: app.RunResult{
					URL:            "https://github.com/gookit/gitw/releases/download/v0.3.6/chlog-windows-amd64.exe",
					Tool:           "gitw",
					ExtractedFiles: []string{"C:/Users/inhere/.local/bin/chlog.exe"},
				},
			},
			Store:  &fakeInstalledStoreForCLI{},
			Config: &fakeConfigRecorderForCLI{},
			Now: func() time.Time {
				return time.Unix(1710000000, 0)
			},
			LoadConfig: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
		},
	}

	err = svc.handle("install", &InstallOptions{
		Targets: []string{"gookit/gitw"},
		Add:     true,
		Name:    "chlog",
	})
	if err != nil {
		t.Fatalf("handle install: %v", err)
	}

	_ = w.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	if !strings.Contains(out.String(), "Added package config: chlog -> gookit/gitw") {
		t.Fatalf("expected add-package message, got %q", out.String())
	}
}

func TestHandleAddPrintsInferredPackageName(t *testing.T) {
	var saved *cfgpkg.File
	svc := &cliService{
		cfgService: app.ConfigService{
			Load: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
			Save: func(path string, file *cfgpkg.File) error {
				saved = file
				return nil
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("add", &AddOptions{Target: "sharkdp/fd"})
	if err != nil {
		t.Fatalf("handle add: %v", err)
	}

	if saved == nil {
		t.Fatal("expected config to be saved")
	}
	if _, ok := saved.Packages["fd"]; !ok {
		t.Fatalf("expected packages.fd to be saved, got %#v", saved.Packages)
	}
	if !strings.Contains(out.String(), "Added package config: fd -> sharkdp/fd") {
		t.Fatalf("expected inferred package name in output, got %q", out.String())
	}
}

func TestHandleInstallAcceptsManagedPackageName(t *testing.T) {
	svc := &cliService{
		appService: app.Service{
			Runner: &fakeRunnerForCLI{
				result: app.RunResult{
					URL:            "https://github.com/sipeed/picoclaw/releases/download/v1.2.3/picoclaw.zip",
					Tool:           "picoclaw",
					ExtractedFiles: []string{"D:/Program/AITools/PicoClaw/picoclaw.exe"},
				},
			},
			Store: &fakeInstalledStoreForCLI{},
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["picoclaw"] = cfgpkg.Section{
					Repo:   util.StringPtr("sipeed/picoclaw"),
					Target: util.StringPtr("D:/Program/AITools/PicoClaw"),
				}
				return cfg, nil
			},
		},
	}

	err := svc.handle("install", &InstallOptions{
		Targets: []string{"picoclaw"},
	})
	if err != nil {
		t.Fatalf("handle install: %v", err)
	}
}

func TestHandleDownloadAcceptsManagedTemplatePackageName(t *testing.T) {
	runner := &fakeRunnerForCLI{
		result: app.RunResult{
			URL:   "https://example.com/1.2.3/win32-x64/claude.exe",
			Tool:  "claude",
			Asset: "claude.exe",
		},
	}
	svc := &cliService{
		appService: app.Service{
			Runner: runner,
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["claude"] = cfgpkg.Section{
					Repo:         util.StringPtr("template:claude"),
					LatestURL:    util.StringPtr("https://example.com/latest"),
					LatestFormat: util.StringPtr("text"),
					URLTemplate:  util.StringPtr("https://example.com/{version}/{os}-{arch}/claude{ext}"),
					OSMap:        map[string]string{"windows": "win32"},
					ArchMap:      map[string]string{"amd64": "x64"},
					ExtMap:       map[string]string{"windows": ".exe"},
				}
				return cfg, nil
			},
		},
	}

	err := svc.handle("download", &DownloadOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("handle download: %v", err)
	}

	assert.Eq(t, []string{"template:claude"}, runner.targets)
	opts := runner.optsByTarget["template:claude"]
	assert.True(t, opts.DownloadOnly)
	assert.Eq(t, "https://example.com/latest", opts.URLTemplate.LatestURL)
	assert.Eq(t, "https://example.com/{version}/{os}-{arch}/claude{ext}", opts.URLTemplate.URLTemplate)
}

func TestHandleInstallInstallsMultipleTargets(t *testing.T) {
	runner := &fakeRunnerForCLI{
		result: app.RunResult{
			URL:            "https://github.com/example/repo/releases/download/v1.0.0/tool.tar.gz",
			Tool:           "tool",
			ExtractedFiles: []string{"./tool"},
		},
	}
	svc := &cliService{
		appService: app.Service{
			Runner: runner,
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
				cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
				return cfg, nil
			},
		},
	}

	err := svc.handle("install", &InstallOptions{Targets: []string{"fzf", "rg"}, Quiet: true})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"junegunn/fzf", "BurntSushi/ripgrep"}, runner.targets)
	if !runner.optsByTarget["junegunn/fzf"].Quiet || !runner.optsByTarget["BurntSushi/ripgrep"].Quiet {
		t.Fatalf("expected cli install options to propagate, got %#v", runner.optsByTarget)
	}
}

func TestHandleUpdateUpdatesMultipleTargets(t *testing.T) {
	installer := &fakeUpdateInstallerForCLI{}
	svc := &cliService{
		updService: app.UpdateService{
			Install: installer,
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
				cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{Installed: map[string]storepkg.Entry{
					"junegunn/fzf":       {Repo: "junegunn/fzf", Tag: "v0.50.0"},
					"BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", Tag: "v13.0.0"},
				}}, nil
			},
			LatestInfo: func(target app.LatestCheckTarget) (app.LatestInfo, error) {
				repo := target.Repo
				switch repo {
				case "junegunn/fzf":
					return app.LatestInfo{Tag: "v0.51.0"}, nil
				case "BurntSushi/ripgrep":
					return app.LatestInfo{Tag: "v14.0.0"}, nil
				default:
					t.Fatalf("unexpected latest check for %s", repo)
					return app.LatestInfo{}, nil
				}
			},
		},
	}

	err := svc.handle("update", &UpdateOptions{Targets: []string{"fzf", "rg"}, Quiet: true})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"fzf", "rg"}, installer.targets)
	if len(installer.options) != 2 || !installer.options[0].Quiet || !installer.options[1].Quiet {
		t.Fatalf("expected cli update options to propagate, got %#v", installer.options)
	}
}

func TestHandleInstallAllInstallsManagedPackages(t *testing.T) {
	runner := &fakeRunnerForCLI{
		result: app.RunResult{
			URL:            "https://github.com/example/repo/releases/download/v1.0.0/tool.tar.gz",
			Tool:           "tool",
			ExtractedFiles: []string{"./tool"},
		},
	}
	svc := &cliService{
		appService: app.Service{
			Runner: runner,
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
				cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
				return cfg, nil
			},
		},
	}

	err := svc.handle("install", &InstallOptions{InstallAll: true, Quiet: true})
	if err != nil {
		t.Fatalf("handle install --all: %v", err)
	}

	assert.Eq(t, []string{"junegunn/fzf", "BurntSushi/ripgrep"}, runner.targets)
	if !runner.optsByTarget["junegunn/fzf"].Quiet || !runner.optsByTarget["BurntSushi/ripgrep"].Quiet {
		t.Fatalf("expected cli install options to propagate, got %#v", runner.optsByTarget)
	}
}

func TestHandleInstallRejectsMissingTargetWithoutAll(t *testing.T) {
	svc := &cliService{}

	err := svc.handle("install", &InstallOptions{})
	if err == nil {
		t.Fatal("expected missing install target to fail")
	}
	if !strings.Contains(err.Error(), "install target is required") {
		t.Fatalf("expected missing target error, got %v", err)
	}
}

func TestHandleRejectsBatchWithoutAll(t *testing.T) {
	svc := &cliService{}

	err := svc.handle("install", &InstallOptions{Targets: []string{"owner/repo"}, BatchConcurrency: 2})
	if err == nil || !strings.Contains(err.Error(), "--batch can only be used with --all") {
		t.Fatalf("expected install --batch without --all to fail, got %v", err)
	}

	err = svc.handle("update", &UpdateOptions{Targets: []string{"fd"}, BatchConcurrency: 2})
	if err == nil || !strings.Contains(err.Error(), "--batch can only be used with --all") {
		t.Fatalf("expected update --batch without --all to fail, got %v", err)
	}
}

func TestInstallOptionsFromCommandsPropagateConcurrency(t *testing.T) {
	installOpts := installOptionsFromInstall(&InstallOptions{
		ChunkConcurrency: 4,
		BatchConcurrency: 2,
	})
	assert.Eq(t, 4, installOpts.ChunkConcurrency)
	assert.Eq(t, 2, installOpts.BatchConcurrency)
	assert.True(t, installOpts.ChunkConcurrencySet)
	assert.True(t, installOpts.BatchConcurrencySet)

	downloadOpts := installOptionsFromDownload(&DownloadOptions{ChunkConcurrency: 3})
	assert.Eq(t, 3, downloadOpts.ChunkConcurrency)
	assert.True(t, downloadOpts.ChunkConcurrencySet)

	addOpts := installOptionsFromAdd(&AddOptions{ChunkConcurrency: 5})
	assert.Eq(t, 5, addOpts.ChunkConcurrency)
	assert.True(t, addOpts.ChunkConcurrencySet)

	updateOpts := installOptionsFromUpdate(&UpdateOptions{
		ChunkConcurrency: 6,
		BatchConcurrency: 4,
	})
	assert.Eq(t, 6, updateOpts.ChunkConcurrency)
	assert.Eq(t, 4, updateOpts.BatchConcurrency)
	assert.True(t, updateOpts.ChunkConcurrencySet)
	assert.True(t, updateOpts.BatchConcurrencySet)
}

func TestHandleInstallAllRejectsTarget(t *testing.T) {
	svc := &cliService{}

	err := svc.handle("install", &InstallOptions{InstallAll: true, Targets: []string{"junegunn/fzf"}})
	if err == nil {
		t.Fatal("expected install --all with target to fail")
	}
	if !strings.Contains(err.Error(), "install --all cannot be used with target") {
		t.Fatalf("expected all with target error, got %v", err)
	}
}

func TestHandleInstallWarnsWhenSudoUserConfigExistsButCurrentConfigMissing(t *testing.T) {
	var stderr bytes.Buffer
	runner := &fakeRunnerForCLI{}
	svc := &cliService{
		stderr: &stderr,
		appService: app.Service{
			Runner: runner,
			LoadConfig: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
		},
		configPathResolver: func() (string, error) {
			return "", os.ErrNotExist
		},
		lookupEnv: func(key string) (string, bool) {
			switch key {
			case "SUDO_USER":
				return "alice", true
			default:
				return "", false
			}
		},
		lookupUserHome: func(name string) (string, error) {
			if name != "alice" {
				t.Fatalf("unexpected user lookup %q", name)
			}
			return "/home/alice", nil
		},
		fileExists: func(path string) bool {
			return filepath.ToSlash(path) == "/home/alice/.config/eget/eget.toml"
		},
	}

	err := svc.handle("install", &InstallOptions{Targets: []string{"babarot/gomi"}})
	if err != nil {
		t.Fatalf("handle install: %v", err)
	}

	got := stderr.String()
	if !strings.Contains(got, "sudo may be using a different HOME") {
		t.Fatalf("expected sudo config warning, got %q", got)
	}
	if !strings.Contains(got, `sudo EGET_CONFIG="/home/alice/.config/eget/eget.toml" eget install`) {
		t.Fatalf("expected workaround in warning, got %q", got)
	}
}

func TestHandleInstallDoesNotWarnWhenQuiet(t *testing.T) {
	var stderr bytes.Buffer
	svc := &cliService{
		stderr: &stderr,
		appService: app.Service{
			Runner: &fakeRunnerForCLI{},
			LoadConfig: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
		},
		configPathResolver: func() (string, error) {
			return "", os.ErrNotExist
		},
		lookupEnv: func(key string) (string, bool) {
			if key == "SUDO_USER" {
				return "alice", true
			}
			return "", false
		},
		lookupUserHome: func(string) (string, error) {
			return "/home/alice", nil
		},
		fileExists: func(path string) bool {
			return path == "/home/alice/.config/eget/eget.toml"
		},
	}

	err := svc.handle("install", &InstallOptions{Targets: []string{"babarot/gomi"}, Quiet: true})
	if err != nil {
		t.Fatalf("handle install: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected quiet install to suppress warning, got %q", stderr.String())
	}
}

func TestHandleInstallDoesNotWarnWhenEGETConfigIsExplicit(t *testing.T) {
	var stderr bytes.Buffer
	svc := &cliService{
		stderr: &stderr,
		appService: app.Service{
			Runner: &fakeRunnerForCLI{},
			LoadConfig: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
		},
		configPathResolver: func() (string, error) {
			return "", os.ErrNotExist
		},
		lookupEnv: func(key string) (string, bool) {
			switch key {
			case "EGET_CONFIG":
				return "/home/alice/.config/eget/eget.toml", true
			case "SUDO_USER":
				return "alice", true
			default:
				return "", false
			}
		},
		lookupUserHome: func(string) (string, error) {
			return "/home/alice", nil
		},
		fileExists: func(path string) bool {
			return path == "/home/alice/.config/eget/eget.toml"
		},
	}

	err := svc.handle("install", &InstallOptions{Targets: []string{"babarot/gomi"}})
	if err != nil {
		t.Fatalf("handle install: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected explicit EGET_CONFIG to suppress warning, got %q", stderr.String())
	}
}

func TestNewCLIServiceWiresReleaseInfo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))

	svc, err := newCLIService()
	if err != nil {
		t.Fatalf("newCLIService: %v", err)
	}
	if svc.appService.ReleaseInfo == nil {
		t.Fatal("expected ReleaseInfo to be configured")
	}
	sdkService, ok := svc.sdkService.(sdk.Service)
	if !ok {
		t.Fatalf("expected sdk.Service, got %T", svc.sdkService)
	}
	if sdkService.Config == nil {
		t.Fatal("expected sdk service config to be configured")
	}
	if sdkService.Store.Path == "" {
		t.Fatal("expected sdk installed store path to be configured")
	}
	if sdkService.IndexCache.Dir == "" {
		t.Fatal("expected sdk index cache dir to be configured")
	}
}

func TestNewCLIServiceLoadsDotenvBeforeConfig(t *testing.T) {
	tmp := t.TempDir()
	xdgHome := filepath.Join(tmp, ".config")
	configDir := filepath.Join(xdgHome, "eget")
	writeCLIFile(t, filepath.Join(configDir, ".env"), "PROXY_URL=http://127.0.0.1:7890\nEGET_SELF_UPDATE_SOURCE=https://example.com/tools/eget/\n")
	writeCLIFile(t, filepath.Join(configDir, "eget.toml"), `
[global]
proxy_url = "${PROXY_URL}"
`)
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("XDG_CONFIG_HOME", xdgHome)
	t.Setenv("PROXY_URL", "")
	t.Setenv("EGET_SELF_UPDATE_SOURCE", "")

	svc, err := newCLIService()

	assert.NoErr(t, err)
	assert.Eq(t, "http://127.0.0.1:7890", svc.proxyURL)
	assert.Eq(t, "https://example.com/tools/eget/", os.Getenv("EGET_SELF_UPDATE_SOURCE"))
}

func TestConfigureVerboseUpdatesVerboseLoggers(t *testing.T) {
	var out bytes.Buffer
	configureVerbose(true, &out)
	if !install.VerboseEnabledForTest() {
		t.Fatalf("expected install verbose to be enabled")
	}
	if !sourcegithub.VerboseEnabledForTest() {
		t.Fatalf("expected source verbose to be enabled")
	}
	if !sourcesf.VerboseEnabledForTest() {
		t.Fatalf("expected sourceforge verbose to be enabled")
	}
	if !forge.VerboseEnabledForTest() {
		t.Fatalf("expected forge verbose to be enabled")
	}
	configureVerbose(false, &out)
}

func TestHandleSDKInstallPrintsResults(t *testing.T) {
	fake := &fakeSDKService{
		installResults: []sdk.InstallResult{{
			Name: "go", Version: "1.21.1", Path: "/sdks/go1.21.1", Cached: true, Resumed: true,
		}},
	}
	svc := &cliService{sdkService: fake}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.install", &SDKInstallOptions{Targets: []string{"go@1.21.1"}, Force: true})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"go@1.21.1"}, fake.installTargets)
	assert.True(t, fake.installOpts.Force)
	assert.NotNil(t, fake.installOpts.Progress)
	got := out.String()
	assert.Contains(t, got, "Install SDK go@1.21.1 -> 1.21.1 from example.com")
	assert.Contains(t, got, "go@1.21.1")
	assert.Contains(t, got, "/sdks/go1.21.1")
	assert.Contains(t, got, "cached")
	assert.Contains(t, got, "resumed")
}

func TestHandleSDKListJSONOutput(t *testing.T) {
	fake := &fakeSDKService{
		listEntries: []sdk.InstalledEntry{{
			Name: "go", Version: "1.21.1", Path: "/sdks/go1.21.1", InstalledAt: time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC),
		}},
	}
	svc := &cliService{sdkService: fake}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	assert.NoErr(t, err)
	defer r.Close()
	defer w.Close()
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	err = svc.handle("sdk.list", &SDKListOptions{Name: "go", JSON: true})
	assert.NoErr(t, err)
	assert.Eq(t, "go", fake.listName)

	_ = w.Close()
	var out bytes.Buffer
	_, err = io.Copy(&out, r)
	assert.NoErr(t, err)
	got := out.String()
	assert.Contains(t, got, `"name": "go"`)
	assert.Contains(t, got, `"installed_at": "2026-05-17T09:00:00"`)
}

func TestHandleSDKRemovePrintsResult(t *testing.T) {
	fake := &fakeSDKService{
		removeResult: sdk.RemoveResult{Name: "go", Version: "1.21.1", Path: "/sdks/go1.21.1", Missing: true},
	}
	svc := &cliService{sdkService: fake}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.remove", &SDKRemoveOptions{Target: "go@1.21.1"})
	assert.NoErr(t, err)
	assert.Eq(t, "go@1.21.1", fake.removeTarget)
	assert.Contains(t, out.String(), "already missing")
}

func TestHandleUninstallRequiresConfirmation(t *testing.T) {
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	assert.NoErr(t, err)
	defer r.Close()
	defer w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	_, err = w.WriteString("n\n")
	assert.NoErr(t, err)
	assert.NoErr(t, w.Close())

	err = (&cliService{}).handle("uninstall", &UninstallOptions{Target: "gookit/gitw"})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "remove cancelled")
}

func TestHandleUninstallYesSkipsConfirmation(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "gitw")
	assert.NoErr(t, os.WriteFile(bin, []byte("gitw"), 0o644))
	store := &fakeUninstallStoreForCLI{
		cfg: &storepkg.Config{Installed: map[string]storepkg.Entry{
			"gookit/gitw": {Repo: "gookit/gitw", ExtractedFiles: []string{bin}},
		}},
	}
	svc := &cliService{
		uninstallService: app.UninstallService{
			Store: store,
			LoadConfig: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
		},
	}

	err := svc.handle("uninstall", &UninstallOptions{Target: "gookit/gitw", Yes: true})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"gookit/gitw"}, store.removeKeys)
}

func TestHandleSDKSearchPrintsResults(t *testing.T) {
	fake := &fakeSDKService{
		searchResults: []sdk.SearchResult{{
			SDK: "go", Version: "1.22.0", Stable: true, OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "go1.22.0.linux-amd64.tar.gz",
		}},
	}
	svc := &cliService{sdkService: fake}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.search", &SDKSearchOptions{Name: "go", Keywords: []string{"1.22", "amd64", "^windows"}, Number: 7, Sort: "desc"})
	assert.NoErr(t, err)
	assert.Eq(t, "go", fake.searchName)
	assert.Eq(t, []string{"1.22", "amd64", "^windows"}, fake.searchKeywords)
	assert.Eq(t, 7, fake.searchNumber)
	assert.Eq(t, "desc", fake.searchSort)
	got := out.String()
	assert.Contains(t, got, "1.22.0")
	assert.Contains(t, got, "linux")
	assert.Contains(t, got, "go1.22.0.linux-amd64.tar.gz")
}

func TestHandleSDKIndexActions(t *testing.T) {
	now := time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC)
	fake := &fakeSDKService{
		index: sdk.Index{
			Schema:    1,
			SDK:       "go",
			SourceURL: "https://example.com/go",
			FetchedAt: now,
			Items: []sdk.IndexItem{
				{Version: "1.21.1", Stable: true, Files: []sdk.IndexFile{
					{OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "go1.21.1.linux-amd64.tar.gz"},
					{OS: "windows", Arch: "amd64", Ext: "zip", Filename: "go1.21.1.windows-amd64.zip"},
				}},
				{Version: "1.22.0-rc.1", Stable: false, Files: []sdk.IndexFile{
					{OS: "linux", Arch: "amd64", Ext: "tar.gz", Filename: "go1.22.0-rc.1.linux-amd64.tar.gz"},
				}},
			},
		},
		indexes: []sdk.Index{
			{Schema: 1, SDK: "go", SourceURL: "https://example.com/go", FetchedAt: now},
		},
		cachedIndexes: []sdk.CachedIndexInfo{
			{SDK: "go", Versions: 3, SourceURL: "https://example.com/sdks/go/releases/download/archive/index/list/with/a/very/long/path", FetchedAt: now, Cached: true},
		},
	}
	svc := &cliService{sdkService: fake}

	assert.NoErr(t, svc.handle("sdk.index.refresh", &SDKIndexOptions{Action: "refresh", Name: "go"}))
	assert.Eq(t, "go", fake.indexName)
	assert.NoErr(t, svc.handle("sdk.index.refresh", &SDKIndexOptions{Action: "refresh", All: true}))
	assert.True(t, fake.indexAll)
	assert.NoErr(t, svc.handle("sdk.index.clear", &SDKIndexOptions{Action: "clear", Name: "go"}))
	assert.Eq(t, "go", fake.clearName)
	assert.NoErr(t, svc.handle("sdk.index.clear", &SDKIndexOptions{Action: "clear", All: true}))
	assert.True(t, fake.clearAll)

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)
	assert.NoErr(t, svc.handle("sdk.index.list", &SDKIndexOptions{Action: "list"}))
	listOut := ccolor.ClearCode(out.String())
	assert.Contains(t, listOut, "go")
	assert.Contains(t, listOut, "...")
	assert.NotContains(t, listOut, "/with/a/very/long/path")

	out.Reset()
	assert.NoErr(t, svc.handle("sdk.index.show", &SDKIndexOptions{Action: "show", Name: "go"}))
	showOut := out.String()
	assert.Contains(t, showOut, "SDK Index")
	assert.Contains(t, showOut, "go")
	assert.Contains(t, showOut, "Versions")
	assert.Contains(t, showOut, "Latest Stable")
	assert.NotContains(t, showOut, " true ")
	assert.NotContains(t, showOut, " false ")
	assert.NotContains(t, showOut, "Version | Stable | Files")
	assert.NotContains(t, showOut, `"items"`)
}

func TestHandleSDKIndexListShowsConfiguredMissingCache(t *testing.T) {
	fake := &fakeSDKService{
		cachedIndexes: []sdk.CachedIndexInfo{
			{SDK: "go", SourceURL: "https://go.dev/dl/"},
		},
	}
	svc := &cliService{sdkService: fake}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	assert.NoErr(t, svc.handle("sdk.index.list", &SDKIndexOptions{Action: "list"}))
	got := ccolor.ClearCode(out.String())
	assert.Contains(t, got, "go")
	assert.Contains(t, got, "https://go.dev/dl/")
	assert.Contains(t, got, " - ")
}

func TestHandleSDKConfigAddPrintsResult(t *testing.T) {
	cfg := cfgpkg.NewFile()
	svc := &cliService{
		cfgService: app.ConfigService{
			Load: func() (*cfgpkg.File, error) { return cfg, nil },
			Save: func(path string, file *cfgpkg.File) error { return nil },
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.config.add", &SDKConfigOptions{Action: "add", Name: "jdk", Mirror: "zulu"})
	if err != nil {
		t.Fatalf("handle sdk config add: %v", err)
	}

	got := ccolor.ClearCode(out.String())
	assert.Contains(t, got, "Added SDK config: jdk (zulu)")
	assert.Eq(t, "zulu-json", *cfg.SDK["jdk"].IndexParser)
}

func TestHandleSDKConfigAddAllPrintsSkippedAndAdded(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.SDK["go"] = cfgpkg.SDKSection{Target: cliStringPtr("custom-go")}
	svc := &cliService{
		cfgService: app.ConfigService{
			Load: func() (*cfgpkg.File, error) { return cfg, nil },
			Save: func(path string, file *cfgpkg.File) error { return nil },
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("sdk.config.add", &SDKConfigOptions{Action: "add", All: true, Mirror: "mirror"})
	if err != nil {
		t.Fatalf("handle sdk config add all: %v", err)
	}

	got := ccolor.ClearCode(out.String())
	assert.Contains(t, got, "Skipped SDK config: go already exists")
	assert.Contains(t, got, "Added SDK config: node (mirror)")
	assert.Contains(t, got, "Added SDK config: jdk (mirror)")
}

func TestHandleListOutdatedPrintsOnlyOutdatedInstalledPackages(t *testing.T) {
	svc := &cliService{
		listService: app.ListService{
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{
					Installed: map[string]storepkg.Entry{
						"BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", Tag: "v13.0.0"},
						"junegunn/fzf":       {Repo: "junegunn/fzf", Tag: "v0.50.0"},
					},
				}, nil
			},
		},
	}
	publishedAt := time.Date(2026, 4, 21, 14, 10, 17, 0, time.UTC)
	svc.listService.LatestInfo = func(target app.LatestCheckTarget) (app.LatestInfo, error) {
		repo := target.Repo
		switch repo {
		case "BurntSushi/ripgrep":
			return app.LatestInfo{Tag: "v14.0.0", PublishedAt: publishedAt}, nil
		case "junegunn/fzf":
			return app.LatestInfo{Tag: "v0.50.0"}, nil
		default:
			return app.LatestInfo{}, nil
		}
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleList(&ListOptions{Outdated: true})
	if err != nil {
		t.Fatalf("handle list outdated: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Outdated Packages:") {
		t.Fatalf("expected outdated packages title, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "latest version") {
		t.Fatalf("expected last_version column in output, got %q", got)
	}
	if !strings.Contains(got, "Published At") {
		t.Fatalf("expected published at column in output, got %q", got)
	}
	if !strings.Contains(got, "2026-04-21T14:10:17") {
		t.Fatalf("expected published time in output, got %q", got)
	}
	if strings.Contains(got, "2026-04-21T14:10:17Z") {
		t.Fatalf("expected published time without timezone offset, got %q", got)
	}
	if strings.Contains(got, " yes ") || strings.Contains(got, " no ") {
		t.Fatalf("expected Installed column values to be removed, got %q", got)
	}
	if !strings.Contains(got, "BurntSushi/ripgrep") {
		t.Fatalf("expected outdated repo in output, got %q", got)
	}
	if !strings.Contains(got, "v14.0.0") {
		t.Fatalf("expected latest_tag in output, got %q", got)
	}
	if strings.Contains(got, "junegunn/fzf") {
		t.Fatalf("expected up-to-date repo to be omitted, got %q", got)
	}
}

func TestHandleListOutdatedPrintsCheckedInstalledCountWhenNothingOutdated(t *testing.T) {
	svc := &cliService{
		listService: app.ListService{
			LatestInfo: func(target app.LatestCheckTarget) (app.LatestInfo, error) {
				repo := target.Repo
				switch repo {
				case "gookit/gitw":
					return app.LatestInfo{Tag: "v0.3.6"}, nil
				case "sipeed/picoclaw":
					return app.LatestInfo{Tag: "v0.2.7"}, nil
				case "windirstat/windirstat":
					return app.LatestInfo{Tag: "release/v2.5.0"}, nil
				default:
					return app.LatestInfo{}, nil
				}
			},
			LoadConfig: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{
					Installed: map[string]storepkg.Entry{
						"gookit/gitw":           {Repo: "gookit/gitw", Tag: "v0.3.6"},
						"sipeed/picoclaw":       {Repo: "sipeed/picoclaw", Tag: "v0.2.7"},
						"windirstat/windirstat": {Repo: "windirstat/windirstat", Tag: "release/v2.5.0"},
					},
				}, nil
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleList(&ListOptions{Outdated: true})
	if err != nil {
		t.Fatalf("handle list outdated: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Checked 3 packages") {
		t.Fatalf("expected checked count for all installed packages, got %q", got)
	}
	if !strings.Contains(got, "No outdated packages found") {
		t.Fatalf("expected no outdated message, got %q", got)
	}
}

func TestHandleListOutdatedPrintsSingleProxyNoticeAndCacheSummary(t *testing.T) {
	cacheDir := t.TempDir()
	repos := []string{"gookit/gitw", "sipeed/picoclaw", "windirstat/windirstat"}
	for _, repo := range repos {
		apiURL := "https://api.github.com/repos/" + repo + "/releases/latest"
		cachePath := install.APICacheFilePath(cacheDir, apiURL)
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
			t.Fatalf("mkdir cache dir: %v", err)
		}
		if err := os.WriteFile(cachePath, []byte(`{"tag_name":"v1.0.0"}`), 0o644); err != nil {
			t.Fatalf("write cache file: %v", err)
		}
	}

	svc := &cliService{
		proxyURL: "http://127.0.0.1:1081",
		listService: app.ListService{
			LatestInfo: func(target app.LatestCheckTarget) (app.LatestInfo, error) {
				repo := target.Repo
				apiURL := "https://api.github.com/repos/" + repo + "/releases/latest"
				resp, err := client.GetWithOptions(apiURL, client.Options{
					ProxyURL:        "http://127.0.0.1:1081",
					APICacheEnabled: true,
					APICacheDir:     cacheDir,
					APICacheTime:    300,
				})
				if err != nil {
					return app.LatestInfo{}, err
				}
				_ = resp.Body.Close()
				return app.LatestInfo{Tag: "v1.0.0"}, nil
			},
			LoadConfig: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{Installed: map[string]storepkg.Entry{
					"gookit/gitw":           {Repo: "gookit/gitw", Tag: "v1.0.0"},
					"sipeed/picoclaw":       {Repo: "sipeed/picoclaw", Tag: "v1.0.0"},
					"windirstat/windirstat": {Repo: "windirstat/windirstat", Tag: "v1.0.0"},
				}}, nil
			},
		},
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	svc.stderr = &stderr
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleList(&ListOptions{Outdated: true})
	if err != nil {
		t.Fatalf("handle list outdated: %v", err)
	}

	gotErr := ccolor.ClearCode(stderr.String())
	if strings.Count(gotErr, "proxy_url for GitHub API request") != 1 {
		t.Fatalf("expected one proxy notice, got %q", gotErr)
	}
	if !strings.Contains(gotErr, "Reused api_cache files: 3") {
		t.Fatalf("expected cache summary, got %q", gotErr)
	}
}

func TestHandleListPrintsOnlyInstalledPackagesByDefault(t *testing.T) {
	now := time.Date(2026, 5, 5, 13, 20, 19, 0, time.FixedZone("CST", 8*60*60))
	svc := &cliService{
		listService: app.ListService{
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["chlog"] = cfgpkg.Section{Repo: util.StringPtr("gookit/gitw")}
				cfg.Packages["claude"] = cfgpkg.Section{Repo: util.StringPtr("template:claude")}
				cfg.Packages["ripgrep"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{
					Installed: map[string]storepkg.Entry{
						"gookit/gitw": {Repo: "gookit/gitw", Tag: "v0.3.6", InstalledAt: now},
						"claude":      {Repo: "template:claude", Target: "template:claude", Tag: "1.2.3", InstalledAt: now},
					},
				}, nil
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleList(&ListOptions{})
	if err != nil {
		t.Fatalf("handle list: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Installed Packages:") {
		t.Fatalf("expected installed packages title, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "name") || !strings.Contains(strings.ToLower(got), "version") {
		t.Fatalf("expected table headers in output, got %q", got)
	}
	if strings.Contains(got, " yes ") || strings.Contains(got, " no ") {
		t.Fatalf("expected Installed column values to be removed, got %q", got)
	}
	if !strings.Contains(got, "Source") || !strings.Contains(got, "Update Time") {
		t.Fatalf("expected Source and Update Time columns, got %q", got)
	}
	if !strings.Contains(got, "chlog") || !strings.Contains(got, "v0.3.6") {
		t.Fatalf("expected table row in output, got %q", got)
	}
	if !strings.Contains(got, "claude") || !strings.Contains(got, "template") {
		t.Fatalf("expected template package source in output, got %q", got)
	}
	if !strings.Contains(got, "github") || !strings.Contains(got, "2026-05-05T13:20:19") {
		t.Fatalf("expected source and update time in output, got %q", got)
	}
	if strings.Contains(got, "2026-05-05T13:20:19+08:00") {
		t.Fatalf("expected update time without timezone offset, got %q", got)
	}
	if strings.Contains(got, "ripgrep") {
		t.Fatalf("expected default list to omit managed-only package, got %q", got)
	}
}

func TestHandleListAllPrintsManagedAndInstalledPackages(t *testing.T) {
	now := time.Date(2026, 5, 5, 13, 20, 19, 0, time.FixedZone("CST", 8*60*60))
	svc := &cliService{
		listService: app.ListService{
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["chlog"] = cfgpkg.Section{Repo: util.StringPtr("gookit/gitw")}
				cfg.Packages["ripgrep"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{
					Installed: map[string]storepkg.Entry{
						"gookit/gitw": {Repo: "gookit/gitw", Tag: "v0.3.6", InstalledAt: now},
					},
				}, nil
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleList(&ListOptions{All: true})
	if err != nil {
		t.Fatalf("handle list all: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Managed Packages:") {
		t.Fatalf("expected managed packages title, got %q", got)
	}
	if !strings.Contains(got, "Source") || !strings.Contains(got, "Update Time") {
		t.Fatalf("expected Source and Update Time columns, got %q", got)
	}
	if !strings.Contains(got, "chlog") || !strings.Contains(got, "ripgrep") {
		t.Fatalf("expected all list to include installed and managed-only packages, got %q", got)
	}
	if !strings.Contains(got, "v0.3.6") || !strings.Contains(got, "No-Install") {
		t.Fatalf("expected installed and non-installed versions, got %q", got)
	}
	if strings.Contains(got, " yes ") || strings.Contains(got, " no ") {
		t.Fatalf("expected Installed column values to be removed, got %q", got)
	}
	if !strings.Contains(got, "2026-05-05T13:20:19") {
		t.Fatalf("expected update time in output, got %q", got)
	}
	if strings.Contains(got, "2026-05-05T13:20:19+08:00") {
		t.Fatalf("expected update time without timezone offset, got %q", got)
	}
}

func TestHandleListNoInstalledPrintsOnlyManagedMissingPackages(t *testing.T) {
	now := time.Date(2026, 5, 5, 13, 20, 19, 0, time.FixedZone("CST", 8*60*60))
	svc := &cliService{
		listService: app.ListService{
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["chlog"] = cfgpkg.Section{Repo: util.StringPtr("gookit/gitw")}
				cfg.Packages["ripgrep"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{
					Installed: map[string]storepkg.Entry{
						"gookit/gitw": {Repo: "gookit/gitw", Tag: "v0.3.6", InstalledAt: now},
					},
				}, nil
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleList(&ListOptions{NoInstalled: true})
	if err != nil {
		t.Fatalf("handle list no-installed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Not Installed Packages:") {
		t.Fatalf("expected not installed packages title, got %q", got)
	}
	if !strings.Contains(got, "ripgrep") || !strings.Contains(got, "No-Install") {
		t.Fatalf("expected managed-only package in output, got %q", got)
	}
	if strings.Contains(got, "chlog") || strings.Contains(got, "v0.3.6") {
		t.Fatalf("expected installed package to be omitted, got %q", got)
	}
}

func TestHandleListGUIPrintsOnlyGUIPackages(t *testing.T) {
	isGUI := true
	svc := &cliService{
		listService: app.ListService{
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["picoclaw"] = cfgpkg.Section{Repo: util.StringPtr("sipeed/picoclaw"), IsGUI: &isGUI}
				cfg.Packages["chlog"] = cfgpkg.Section{Repo: util.StringPtr("gookit/gitw")}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{Installed: map[string]storepkg.Entry{
					"sipeed/picoclaw": {Repo: "sipeed/picoclaw", Tag: "v0.2.7", IsGUI: true, InstallMode: "portable"},
					"gookit/gitw":     {Repo: "gookit/gitw", Tag: "v0.3.6"},
				}}, nil
			},
		},
	}
	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)
	err := svc.handleList(&ListOptions{GUI: true})
	if err != nil {
		t.Fatalf("handle list gui: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "picoclaw") || strings.Contains(got, "chlog") {
		t.Fatalf("expected only gui package output, got %q", got)
	}
}

func TestHandleListInfoPrintsDetails(t *testing.T) {
	now := time.Date(2026, 5, 5, 13, 20, 19, 0, time.FixedZone("CST", 8*60*60))
	svc := &cliService{
		listService: app.ListService{
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["chlog"] = cfgpkg.Section{
					Repo:   util.StringPtr("gookit/gitw"),
					Target: util.StringPtr("~/.local/bin"),
				}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{
					Installed: map[string]storepkg.Entry{
						"gookit/gitw": {
							Repo:        "gookit/gitw",
							InstalledAt: now,
							Tag:         "v0.3.6",
							Asset:       "chlog-windows-amd64.exe",
							URL:         "https://github.com/gookit/gitw/releases/download/v0.3.6/chlog-windows-amd64.exe",
						},
					},
				}, nil
			},
		},
	}

	var out bytes.Buffer
	cliui.SetOutput(&out)
	defer cliui.ResetOutput()

	err := svc.handleList(&ListOptions{Info: "chlog"})
	if err != nil {
		t.Fatalf("handle list info: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Package Info") || !strings.Contains(got, "Name") || !strings.Contains(got, "chlog") {
		t.Fatalf("expected detail output, got %q", got)
	}
	if !strings.Contains(got, "Version") || !strings.Contains(got, "v0.3.6") {
		t.Fatalf("expected version detail output, got %q", got)
	}
	if !strings.Contains(got, "URL") || !strings.Contains(got, "https://github.com/gookit/gitw/releases/download/v0.3.6/chlog-windows-amd64.exe") {
		t.Fatalf("expected detailed url output, got %q", got)
	}
	if !strings.Contains(got, "2026-05-05T13:20:19") {
		t.Fatalf("expected compact installed time in detail output, got %q", got)
	}
	if strings.Contains(got, "2026-05-05T13:20:19+08:00") || strings.Contains(got, "2026-05-05 13:20:19 +0800") {
		t.Fatalf("expected detail time without timezone offset, got %q", got)
	}
}

func TestHandleShowPrintsPackageDetails(t *testing.T) {
	installedAt := time.Date(2026, 5, 5, 13, 20, 19, 0, time.UTC)
	svc := &cliService{
		showService: app.ShowService{
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["chlog"] = cfgpkg.Section{
					Repo: util.StringPtr("gookit/gitw"),
					Desc: util.StringPtr("Git helper tools"),
				}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{Installed: map[string]storepkg.Entry{
					"gookit/gitw": {
						Repo:           "gookit/gitw",
						Target:         "gookit/gitw",
						InstalledAt:    installedAt,
						Tag:            "v0.3.6",
						Version:        "v0.3.6",
						Asset:          "chlog-windows-amd64.exe",
						URL:            "https://github.com/gookit/gitw/releases/download/v0.3.6/chlog-windows-amd64.exe",
						ExtractedFiles: []string{"D:/bin/chlog.exe"},
						RepoURL:        "https://github.com/gookit/gitw",
					},
				}}, nil
			},
		},
	}

	var out bytes.Buffer
	cliui.SetOutput(&out)
	defer cliui.ResetOutput()

	err := svc.handleShow(&ShowOptions{Target: "chlog"})
	if err != nil {
		t.Fatalf("handle show: %v", err)
	}

	got := out.String()
	assert.Contains(t, got, "Package Details")
	assert.Contains(t, got, "chlog")
	assert.Contains(t, got, "Git helper tools")
	assert.Contains(t, got, "v0.3.6")
	assert.Contains(t, got, "https://github.com/gookit/gitw")
	assert.Contains(t, got, "D:/bin/chlog.exe")
	assert.Contains(t, got, "2026-05-05T13:20:19")
}

func TestHandleListRejectsOutdatedWithInfo(t *testing.T) {
	svc := &cliService{}
	err := svc.handleList(&ListOptions{Outdated: true, Info: "chlog"})
	if err == nil {
		t.Fatal("expected conflicting list options to fail")
	}
}

func TestHandleListRejectsNoInstalledConflicts(t *testing.T) {
	svc := &cliService{}
	if err := svc.handleList(&ListOptions{NoInstalled: true, Info: "chlog"}); err == nil {
		t.Fatal("expected no-installed with info to fail")
	}
	if err := svc.handleList(&ListOptions{NoInstalled: true, Outdated: true}); err == nil {
		t.Fatal("expected no-installed with outdated to fail")
	}
}

func TestHandleUpdateRejectsUnimplementedDryRunAndInteractive(t *testing.T) {
	svc := &cliService{}

	err := svc.handleUpdate(&UpdateOptions{DryRun: true, Targets: []string{"junegunn/fzf"}})
	if err == nil || !strings.Contains(err.Error(), "update --dry-run is not implemented") {
		t.Fatalf("expected dry-run unsupported error, got %v", err)
	}

	err = svc.handleUpdate(&UpdateOptions{Interactive: true, Targets: []string{"junegunn/fzf"}})
	if err == nil || !strings.Contains(err.Error(), "update --interactive is not implemented") {
		t.Fatalf("expected interactive unsupported error, got %v", err)
	}
}

func TestHandleUpdateSelfRejectsTargets(t *testing.T) {
	svc := &cliService{}

	err := svc.handleUpdate(&UpdateOptions{Self: true, Targets: []string{"junegunn/fzf"}})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "update --self cannot be used with target")
}

func TestHandleUpdateSelfRejectsAll(t *testing.T) {
	svc := &cliService{}

	err := svc.handleUpdate(&UpdateOptions{Self: true, All: true})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "update --self cannot be used with --all")
}

func TestHandleUpdateSelfRunsSelfUpdateService(t *testing.T) {
	fake := &fakeSelfUpdateCLIService{
		result: app.SelfUpdateResult{CurrentVersion: "1.7.1", LatestVersion: "v1.7.2", Updated: true},
	}
	svc := &cliService{selfUpdateService: fake, stderr: io.Discard}

	err := svc.handleUpdate(&UpdateOptions{Self: true, Tag: "v1.7.2", Asset: "custom,linux", Quiet: true})

	assert.NoErr(t, err)
	assert.Eq(t, "v1.7.2", fake.opts.Tag)
	assert.Eq(t, []string{"custom", "linux"}, fake.opts.Asset)
	assert.True(t, fake.opts.Install.Quiet)
}

func TestHandleUpdateSelfPassesSourceFromFlag(t *testing.T) {
	fake := &fakeSelfUpdateCLIService{
		result: app.SelfUpdateResult{CurrentVersion: "1.7.1", LatestVersion: "1.7.1-18-gabcd1234", Updated: true},
	}
	svc := &cliService{selfUpdateService: fake, stderr: io.Discard}

	err := svc.handleUpdate(&UpdateOptions{Self: true, SelfSource: "https://example.com/tools/eget/"})

	assert.NoErr(t, err)
	assert.Eq(t, "https://example.com/tools/eget/", fake.opts.Source)
}

func TestHandleUpdateSelfPassesSourceFromEnv(t *testing.T) {
	fake := &fakeSelfUpdateCLIService{
		result: app.SelfUpdateResult{CurrentVersion: "1.7.1", LatestVersion: "1.7.1-18-gabcd1234", Updated: true},
	}
	svc := &cliService{
		selfUpdateService: fake,
		stderr:            io.Discard,
		lookupEnv: func(key string) (string, bool) {
			if key == "EGET_SELF_UPDATE_SOURCE" {
				return "https://example.com/tools/eget/", true
			}
			return "", false
		},
	}

	err := svc.handleUpdate(&UpdateOptions{Self: true})

	assert.NoErr(t, err)
	assert.Eq(t, "https://example.com/tools/eget/", fake.opts.Source)
}

func TestHandleUpdateAllPrintsCandidatesAndUpdatesOnlyOutdated(t *testing.T) {
	installer := &fakeUpdateInstallerForCLI{}
	svc := &cliService{
		updService: app.UpdateService{
			Install: installer,
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
				cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{Installed: map[string]storepkg.Entry{
					"junegunn/fzf":       {Repo: "junegunn/fzf", Tag: "v0.50.0"},
					"BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", Tag: "v13.0.0"},
				}}, nil
			},
			LatestInfo: func(target app.LatestCheckTarget) (app.LatestInfo, error) {
				repo := target.Repo
				switch repo {
				case "junegunn/fzf":
					return app.LatestInfo{Tag: "v0.50.0"}, nil
				case "BurntSushi/ripgrep":
					return app.LatestInfo{Tag: "v14.0.0"}, nil
				default:
					t.Fatalf("unexpected latest tag check for %s", repo)
					return app.LatestInfo{}, nil
				}
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleUpdate(&UpdateOptions{All: true})
	if err != nil {
		t.Fatalf("handle update all: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "BurntSushi/ripgrep") || !strings.Contains(got, "v13.0.0") || !strings.Contains(got, "v14.0.0") {
		t.Fatalf("expected outdated candidate output, got %q", got)
	}
	if strings.Contains(got, "junegunn/fzf") {
		t.Fatalf("expected up-to-date repo to be omitted, got %q", got)
	}
	if len(installer.targets) != 1 || installer.targets[0] != "rg" {
		t.Fatalf("expected only rg to be updated, got %#v", installer.targets)
	}
}

func TestHandleUpdateCheckPrintsSameOutdatedListWithoutUpdating(t *testing.T) {
	installer := &fakeUpdateInstallerForCLI{}
	svc := &cliService{
		listService: app.ListService{
			LatestInfo: func(target app.LatestCheckTarget) (app.LatestInfo, error) {
				repo := target.Repo
				switch repo {
				case "BurntSushi/ripgrep":
					return app.LatestInfo{Tag: "v14.0.0"}, nil
				case "junegunn/fzf":
					return app.LatestInfo{Tag: "v0.50.0"}, nil
				default:
					return app.LatestInfo{}, nil
				}
			},
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
				return cfg, nil
			},
			LoadInstalled: func() (*storepkg.Config, error) {
				return &storepkg.Config{Installed: map[string]storepkg.Entry{
					"BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", Tag: "v13.0.0"},
					"junegunn/fzf":       {Repo: "junegunn/fzf", Tag: "v0.50.0"},
				}}, nil
			},
		},
		updService: app.UpdateService{
			Install: installer,
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleUpdate(&UpdateOptions{Check: true})
	if err != nil {
		t.Fatalf("handle update check: %v", err)
	}

	got := out.String()
	if !strings.Contains(strings.ToLower(got), "latest version") {
		t.Fatalf("expected outdated table output, got %q", got)
	}
	if !strings.Contains(got, "BurntSushi/ripgrep") || !strings.Contains(got, "v14.0.0") {
		t.Fatalf("expected outdated repo in output, got %q", got)
	}
	if strings.Contains(got, "junegunn/fzf") {
		t.Fatalf("expected up-to-date repo to be omitted, got %q", got)
	}
	if len(installer.targets) != 0 {
		t.Fatalf("expected update --check not to update packages, got %#v", installer.targets)
	}
}

func TestHandleConfigInitRejectsOverwriteWithoutConfirmation(t *testing.T) {
	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: "testdata/eget.toml",
			Load: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				target := "~/bin"
				cfg.Global.Target = &target
				return cfg, nil
			},
		},
	}

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	err = svc.handleConfig(&ConfigOptions{Action: "init"})
	if err == nil {
		t.Fatal("expected overwrite rejection error")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestHandleConfigInitTreatsBlankOverwriteConfirmationAsCancel(t *testing.T) {
	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: "testdata/eget.toml",
			Load: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
		},
	}

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	if _, err := w.WriteString("\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	err = svc.handleConfig(&ConfigOptions{Action: "init"})
	if err == nil {
		t.Fatal("expected blank confirmation to cancel")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestHandleConfigInitAllowsOverwriteWithConfirmation(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: configPath,
		},
	}

	if err := os.WriteFile(configPath, []byte("[global]\ntarget = \"~/bin\"\n"), 0o644); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	if _, err := w.WriteString("y\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	if err := svc.handleConfig(&ConfigOptions{Action: "init"}); err != nil {
		t.Fatalf("expected overwrite confirmation to allow init, got %v", err)
	}

	cfg, err := cfgpkg.LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Global.Target == nil || *cfg.Global.Target != "~/.local/bin" {
		t.Fatalf("expected config to be overwritten with defaults, got %#v", cfg.Global.Target)
	}
}

func TestHandleQueryPrintsLatestRelease(t *testing.T) {
	svc := &cliService{
		queryService: app.QueryService{
			Client: &fakeQueryClientForCLI{
				releases: []app.QueryRelease{{
					Tag:         "v1.2.3",
					Name:        "v1.2.3",
					PublishedAt: time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
					AssetsCount: 2,
				}},
			},
		},
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdout = w
	cliui.SetOutput(w)
	defer func() {
		os.Stdout = origStdout
		cliui.ResetOutput()
	}()

	err = svc.handleQuery(&QueryOptions{Target: "owner/repo"})
	if err != nil {
		t.Fatalf("handle query: %v", err)
	}

	_ = w.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "action: latest") || !strings.Contains(got, "repo: owner/repo") {
		t.Fatalf("expected latest query output, got %q", got)
	}
	if !strings.Contains(got, "2026-04-22T09:00:00") {
		t.Fatalf("expected compact published time in latest output, got %q", got)
	}
	if strings.Contains(got, "2026-04-22T09:00:00Z") {
		t.Fatalf("expected latest output without timezone offset, got %q", got)
	}
}

func TestPrintQueryResultReleasesUsesCompactTime(t *testing.T) {
	result := app.QueryResult{
		Action: "releases",
		Repo:   "owner/repo",
		Releases: []app.QueryRelease{{
			Tag:         "v1.2.3",
			Name:        "v1.2.3",
			PublishedAt: time.Date(2026, 4, 22, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
			AssetsCount: 2,
		}},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	printQueryResult(result)
	got := out.String()
	if !strings.Contains(got, "2026-04-22T09:00:00") {
		t.Fatalf("expected compact published time in releases output, got %q", got)
	}
	if strings.Contains(got, "2026-04-22T09:00:00+08:00") {
		t.Fatalf("expected releases output without timezone offset, got %q", got)
	}
}

func TestPrintQueryResultSourceForgeReleasesShowsUnknownAssetsCount(t *testing.T) {
	result := app.QueryResult{
		Action: "releases",
		Repo:   "sourceforge:project/path",
		Releases: []app.QueryRelease{{
			Tag:  "tool-1.2.3",
			Name: "1.2.3",
		}},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	printQueryResult(result)
	got := out.String()
	if !strings.Contains(got, "Assets Count") || !strings.Contains(got, " - ") {
		t.Fatalf("expected unknown assets count marker in sourceforge releases output, got %q", got)
	}
	if strings.Contains(got, " 0 ") {
		t.Fatalf("expected sourceforge releases output to hide zero assets count, got %q", got)
	}
}

func TestPrintQueryResultInfoUsesCompactTime(t *testing.T) {
	result := app.QueryResult{
		Action: "info",
		Repo:   "owner/repo",
		Info: &app.QueryRepoInfo{
			Repo:      "owner/repo",
			UpdatedAt: time.Date(2026, 4, 22, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
		},
	}

	var out bytes.Buffer
	cliui.SetOutput(&out)
	defer cliui.ResetOutput()

	printQueryResult(result)
	got := out.String()
	if !strings.Contains(got, "2026-04-22T09:00:00") {
		t.Fatalf("expected compact updated time in info output, got %q", got)
	}
	if strings.Contains(got, "2026-04-22T09:00:00+08:00") {
		t.Fatalf("expected info output without timezone offset, got %q", got)
	}
}

func TestHandleQueryJSONOutput(t *testing.T) {
	svc := &cliService{
		queryService: app.QueryService{
			Client: &fakeQueryClientForCLI{
				releases: []app.QueryRelease{{
					Tag:         "v1.2.3",
					PublishedAt: time.Date(2026, 4, 22, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
				}},
			},
		},
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	err = svc.handleQuery(&QueryOptions{Target: "owner/repo", JSON: true})
	if err != nil {
		t.Fatalf("handle query: %v", err)
	}

	_ = w.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `"action": "latest"`) {
		t.Fatalf("expected json query output, got %q", got)
	}
	if !strings.Contains(got, `"published_at": "2026-04-22T09:00:00"`) {
		t.Fatalf("expected compact json query time, got %q", got)
	}
	if strings.Contains(got, "2026-04-22T09:00:00+08:00") {
		t.Fatalf("expected json query time without timezone offset, got %q", got)
	}
}

func TestPrintQueryResultAssets(t *testing.T) {
	result := app.QueryResult{
		Action: "assets",
		Repo:   "owner/repo",
		Tag:    "v1.2.3",
		Assets: []app.QueryAsset{{
			Name: "tool-linux-amd64.tar.gz",
			URL:  "https://example.com/tool",
		}},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	printQueryResult(result)
	if !strings.Contains(out.String(), "tool-linux-amd64.tar.gz") {
		t.Fatalf("expected asset table output, got %q", out.String())
	}
}

func TestHandleSearchPrintsList(t *testing.T) {
	svc := &cliService{
		searchService: app.SearchService{
			Client: &fakeSearchClientForCLI{
				result: app.SearchResult{
					TotalCount: 1,
					Items: []app.SearchRepo{{
						FullName:        "BurntSushi/ripgrep",
						Description:     "ripgrep recursively searches directories",
						StargazersCount: 123,
						Language:        "Rust",
						UpdatedAt:       time.Date(2026, 4, 24, 8, 30, 0, 0, time.UTC),
					}},
				},
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handle("search", &SearchOptions{Keyword: "ripgrep", Extras: []string{"language:rust"}})
	if err != nil {
		t.Fatalf("handle search: %v", err)
	}

	got := out.String()
	if strings.Contains(strings.ToLower(got), "repo |") || strings.Contains(strings.ToLower(got), "language |") {
		t.Fatalf("expected search to not render a table, got %q", got)
	}
	if !strings.Contains(got, "BurntSushi/ripgrep") || !strings.Contains(got, "⭐123 language: Rust update: 2026-04-24T08:30:00") {
		t.Fatalf("expected formatted search headline, got %q", got)
	}
	if strings.Contains(got, "2026-04-24T08:30:00Z") {
		t.Fatalf("expected search time without timezone offset, got %q", got)
	}
	if !strings.Contains(got, "ripgrep recursively searches directories") {
		t.Fatalf("expected description line, got %q", got)
	}
}

func TestHandleSearchJSONOutput(t *testing.T) {
	svc := &cliService{
		searchService: app.SearchService{
			Client: &fakeSearchClientForCLI{
				result: app.SearchResult{
					TotalCount: 1,
					Items: []app.SearchRepo{{
						FullName:        "BurntSushi/ripgrep",
						StargazersCount: 321,
						UpdatedAt:       time.Date(2026, 4, 24, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60)),
					}},
				},
			},
		},
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	err = svc.handle("search", &SearchOptions{Keyword: "ripgrep", JSON: true})
	if err != nil {
		t.Fatalf("handle search json: %v", err)
	}

	_ = w.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `"total_count": 1`) || !strings.Contains(got, `"full_name": "BurntSushi/ripgrep"`) {
		t.Fatalf("expected search json output, got %q", got)
	}
	if !strings.Contains(got, `"updated_at": "2026-04-24T08:30:00"`) {
		t.Fatalf("expected compact search json time, got %q", got)
	}
	if strings.Contains(got, "2026-04-24T08:30:00+08:00") {
		t.Fatalf("expected search json time without timezone offset, got %q", got)
	}
}

func TestHandleSearchPassesOptionsToSearchService(t *testing.T) {
	fakeClient := &fakeSearchClientForCLI{
		result: app.SearchResult{},
	}
	svc := &cliService{
		searchService: app.SearchService{
			Client: fakeClient,
		},
	}

	err := svc.handle("search", &SearchOptions{
		Keyword: "ripgrep",
		Limit:   20,
		Sort:    "stars",
		Order:   "desc",
		Extras:  []string{"language:rust", "topic:cli"},
	})
	if err != nil {
		t.Fatalf("handle search: %v", err)
	}

	if fakeClient.query != "ripgrep language:rust topic:cli" {
		t.Fatalf("expected merged query to propagate, got %q", fakeClient.query)
	}
	if fakeClient.limit != 20 {
		t.Fatalf("expected limit to propagate, got %d", fakeClient.limit)
	}
	if fakeClient.sort != "stars" {
		t.Fatalf("expected sort to propagate, got %q", fakeClient.sort)
	}
	if fakeClient.order != "desc" {
		t.Fatalf("expected order to propagate, got %q", fakeClient.order)
	}
}

type fakeRunnerForCLI struct {
	result       app.RunResult
	targets      []string
	optsByTarget map[string]install.Options
}

func (f *fakeRunnerForCLI) Run(target string, opts install.Options) (app.RunResult, error) {
	f.targets = append(f.targets, target)
	if f.optsByTarget == nil {
		f.optsByTarget = map[string]install.Options{}
	}
	f.optsByTarget[target] = opts
	return f.result, nil
}

type fakeUpdateInstallerForCLI struct {
	targets []string
	options []install.Options
}

func (f *fakeUpdateInstallerForCLI) InstallTarget(target string, opts install.Options, extras ...app.InstallExtras) (app.RunResult, error) {
	f.targets = append(f.targets, target)
	f.options = append(f.options, opts)
	return app.RunResult{}, nil
}

type fakeSelfUpdateCLIService struct {
	opts   app.SelfUpdateOptions
	result app.SelfUpdateResult
}

func (f *fakeSelfUpdateCLIService) Update(opts app.SelfUpdateOptions) (app.SelfUpdateResult, error) {
	f.opts = opts
	return f.result, nil
}

type fakeInstalledStoreForCLI struct{}

func (f *fakeInstalledStoreForCLI) Record(target string, entry storepkg.Entry) error {
	return nil
}

type fakeConfigRecorderForCLI struct{}

func (f *fakeConfigRecorderForCLI) AddPackage(repo, name string, opts install.Options) error {
	return nil
}

func cliStringPtr(value string) *string {
	return &value
}

type fakeQueryClientForCLI struct {
	repoInfo QueryRepoInfoAlias
	releases []app.QueryRelease
	assets   []app.QueryAsset
}

type QueryRepoInfoAlias = app.QueryRepoInfo

func (f *fakeQueryClientForCLI) RepoInfo(repo string) (app.QueryRepoInfo, error) {
	info := app.QueryRepoInfo(f.repoInfo)
	if info.Repo == "" {
		info.Repo = repo
	}
	return info, nil
}

func (f *fakeQueryClientForCLI) LatestRelease(repo string, includePrerelease bool) (app.QueryRelease, error) {
	if len(f.releases) == 0 {
		return app.QueryRelease{}, nil
	}
	return f.releases[0], nil
}

func (f *fakeQueryClientForCLI) ListReleases(repo string, limit int, includePrerelease bool) ([]app.QueryRelease, error) {
	return f.releases, nil
}

func (f *fakeQueryClientForCLI) ReleaseAssets(repo, tag string) ([]app.QueryAsset, error) {
	return f.assets, nil
}

type fakeSearchClientForCLI struct {
	result app.SearchResult
	err    error
	query  string
	limit  int
	sort   string
	order  string
}

func (f *fakeSearchClientForCLI) SearchRepositories(query string, limit int, sort, order string) (app.SearchResult, error) {
	f.query = query
	f.limit = limit
	f.sort = sort
	f.order = order
	if f.err != nil {
		return app.SearchResult{}, f.err
	}
	return f.result, nil
}
