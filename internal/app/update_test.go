package app

import (
	"sync"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	storepkg "github.com/inherelab/eget/internal/installed"
	"github.com/inherelab/eget/internal/util"
)

type fakeInstallService struct {
	mu        sync.Mutex
	targets   []string
	options   []install.Options
	result    RunResult
	err       error
	active    int
	maxActive int
	block     chan struct{}
}

func (f *fakeInstallService) InstallTarget(target string, opts install.Options, extras ...InstallExtras) (RunResult, error) {
	f.mu.Lock()
	f.targets = append(f.targets, target)
	f.options = append(f.options, opts)
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	block := f.block
	f.mu.Unlock()

	if block != nil {
		<-block
	}

	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	return f.result, f.err
}

func (f *fakeInstallService) currentMaxActive() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxActive
}

func TestUpdatePackageDelegatesManagedPackageNameWithRawCLIOptions(t *testing.T) {
	installer := &fakeInstallService{}
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Global.Target = util.StringPtr("~/bin")
			cfg.Packages["fzf"] = cfgpkg.Section{
				Repo:   util.StringPtr("junegunn/fzf"),
				Target: util.StringPtr("~/.local/bin"),
				System: util.StringPtr("linux/amd64"),
				Tag:    util.StringPtr("nightly"),
			}
			return cfg, nil
		},
	}

	cli := install.Options{Tag: "v1.0.0", Quiet: true}
	if _, err := svc.UpdatePackage("fzf", cli); err != nil {
		t.Fatalf("update package: %v", err)
	}

	if len(installer.targets) != 1 || installer.targets[0] != "fzf" {
		t.Fatalf("expected installer to resolve managed package name, got %#v", installer.targets)
	}
	if installer.options[0].Output != "" {
		t.Fatalf("expected update service to leave config merging to installer, got output %q", installer.options[0].Output)
	}
	if installer.options[0].Tag != "v1.0.0" || !installer.options[0].Quiet {
		t.Fatalf("expected raw cli options to pass through, got %#v", installer.options[0])
	}
}

func TestUpdatePackageAllowsDirectRepo(t *testing.T) {
	installer := &fakeInstallService{}
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Global.Target = util.StringPtr("~/bin")
			cfg.Repos["junegunn/fzf"] = cfgpkg.Section{System: util.StringPtr("linux/amd64")}
			return cfg, nil
		},
	}

	if _, err := svc.UpdatePackage("junegunn/fzf", install.Options{Tag: "v1.0.0"}); err != nil {
		t.Fatalf("update direct repo: %v", err)
	}

	if len(installer.targets) != 1 || installer.targets[0] != "junegunn/fzf" {
		t.Fatalf("expected installer to use direct repo, got %#v", installer.targets)
	}
	if installer.options[0].Tag != "v1.0.0" {
		t.Fatalf("expected cli tag to win, got %#v", installer.options[0].Tag)
	}
}

func TestUpdatePackageAllowsDirectSourceForgeTarget(t *testing.T) {
	installer := &fakeInstallService{}
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
	}

	_, err := svc.UpdatePackage("sourceforge:winmerge", install.Options{})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"sourceforge:winmerge"}, installer.targets)
}

func TestUpdatePackageAllowsDirectForgeTargets(t *testing.T) {
	installer := &fakeInstallService{}
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
	}

	_, err := svc.UpdatePackage("gitlab:fdroid/fdroidserver", install.Options{})
	assert.NoErr(t, err)
	_, err = svc.UpdatePackage("gitea:codeberg.org/forgejo/forgejo", install.Options{})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"gitlab:fdroid/fdroidserver", "gitea:codeberg.org/forgejo/forgejo"}, installer.targets)
}

func TestUpdatePackageRejectsUnknownPlainWords(t *testing.T) {
	installer := &fakeInstallService{}
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
	}

	for _, name := range []string{"gitlab", "not-managed", "foo/bar/baz"} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.UpdatePackage(name, install.Options{})
			if err == nil || err.Error() != `unknown package "`+name+`"` {
				t.Fatalf("expected unknown package error for %q, got %v", name, err)
			}
		})
	}
	assert.Eq(t, 0, len(installer.targets))
}

func TestUpdatePackageWithAppInstallerKeepsManagedConfigMerge(t *testing.T) {
	cfg := mustLoadFromString(t, `
[global]
target = "~/.local/bin"

["junegunn/fzf"]
system = "linux/amd64"

[packages.fzf]
repo = "junegunn/fzf"
target = "D:/Tools/fzf"
tag = "nightly"
asset_filters = ["linux"]
`)
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://github.com/junegunn/fzf/releases/download/nightly/fzf.tar.gz",
			Tool:           "fzf",
			ExtractedFiles: []string{"./fzf"},
		},
	}
	installSvc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}
	updateSvc := UpdateService{
		Install: installSvc,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
	}

	if _, err := updateSvc.UpdatePackage("fzf", install.Options{}); err != nil {
		t.Fatalf("update package: %v", err)
	}

	if runner.target != "junegunn/fzf" {
		t.Fatalf("expected installer to resolve repo target, got %q", runner.target)
	}
	if runner.opts.Output != "D:/Tools/fzf" {
		t.Fatalf("expected package target to be merged by installer, got %q", runner.opts.Output)
	}
	if runner.opts.System != "linux/amd64" {
		t.Fatalf("expected repo system to be merged by installer, got %q", runner.opts.System)
	}
	if runner.opts.Tag != "nightly" {
		t.Fatalf("expected package tag to be merged by installer, got %q", runner.opts.Tag)
	}
	if len(runner.opts.Asset) != 1 || runner.opts.Asset[0] != "linux" {
		t.Fatalf("expected package asset filters to be merged by installer, got %#v", runner.opts.Asset)
	}
}

func TestUpdateAllPackagesIteratesOutdatedManagedPackages(t *testing.T) {
	installer := &fakeInstallService{}
	svc := UpdateService{
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
		LatestInfo: func(repo, _ string) (LatestInfo, error) {
			switch repo {
			case "junegunn/fzf":
				return LatestInfo{Tag: "v0.51.0"}, nil
			case "BurntSushi/ripgrep":
				return LatestInfo{Tag: "v14.0.0"}, nil
			default:
				return LatestInfo{}, nil
			}
		},
	}

	results, err := svc.UpdateAllPackages(install.Options{})
	if err != nil {
		t.Fatalf("update all packages: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 update results, got %d", len(results))
	}
	if len(installer.targets) != 2 {
		t.Fatalf("expected installer to run twice, got %d", len(installer.targets))
	}
}

func TestUpdateAllPackagesInstallsOnlyOutdatedInstalledPackages(t *testing.T) {
	now := time.Unix(1710000000, 0).UTC()
	installer := &fakeInstallService{}
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
			cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
			cfg.Packages["fd"] = cfgpkg.Section{Repo: util.StringPtr("sharkdp/fd")}
			return cfg, nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{
				"junegunn/fzf":       {Repo: "junegunn/fzf", InstalledAt: now, Tag: "v0.50.0"},
				"BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", InstalledAt: now, Tag: "v13.0.0"},
			}}, nil
		},
		LatestInfo: func(repo, _ string) (LatestInfo, error) {
			switch repo {
			case "junegunn/fzf":
				return LatestInfo{Tag: "v0.50.0"}, nil
			case "BurntSushi/ripgrep":
				return LatestInfo{Tag: "v14.0.0"}, nil
			default:
				t.Fatalf("unexpected latest tag check for %s", repo)
				return LatestInfo{}, nil
			}
		},
	}

	results, err := svc.UpdateAllPackages(install.Options{})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"rg"}, installer.targets)
	assert.Eq(t, 1, len(results))
	assert.Eq(t, "rg", results[0].Name)
	assert.Eq(t, "BurntSushi/ripgrep", results[0].Target)
}

func TestUpdateAllPackagesIgnoresConfiguredPackageNames(t *testing.T) {
	installer := &fakeInstallService{}
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Global.IgnoreUpdatePackages = []string{"fzf"}
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
		LatestInfo: func(repo, _ string) (LatestInfo, error) {
			if repo == "junegunn/fzf" {
				t.Fatal("expected ignored package fzf not to be checked")
			}
			return LatestInfo{Tag: "v14.0.0"}, nil
		},
	}

	results, err := svc.UpdateAllPackages(install.Options{})
	assert.NoErr(t, err)
	assert.Eq(t, []string{"rg"}, installer.targets)
	assert.Eq(t, 1, len(results))
	assert.Eq(t, "rg", results[0].Name)
}

func TestListUpdateCandidatesIgnoresConfiguredPackageNames(t *testing.T) {
	svc := UpdateService{
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Global.IgnoreUpdatePackages = []string{"fzf"}
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
		LatestInfo: func(repo, _ string) (LatestInfo, error) {
			if repo == "junegunn/fzf" {
				t.Fatal("expected ignored package fzf not to be checked")
			}
			return LatestInfo{Tag: "v14.0.0"}, nil
		},
	}

	items, failures, checked, err := svc.ListUpdateCandidates()
	assert.NoErr(t, err)
	assert.Eq(t, 1, checked)
	assert.Eq(t, 0, len(failures))
	assert.Eq(t, 1, len(items))
	assert.Eq(t, "rg", items[0].Name)
}

func TestUpdateAllPackagesPassesAPICacheOptionsFromConfigToInstaller(t *testing.T) {
	installer := &fakeInstallService{}
	cacheTime := 120
	apiCacheEnabled := true
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Global.CacheDir = util.StringPtr("~/.cache/eget")
			cfg.ApiCache.Enable = &apiCacheEnabled
			cfg.ApiCache.CacheTime = &cacheTime
			cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
			return cfg, nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{
				"BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", Tag: "v13.0.0"},
			}}, nil
		},
		LatestInfo: func(repo, _ string) (LatestInfo, error) {
			return LatestInfo{Tag: "v14.0.0"}, nil
		},
	}

	_, err := svc.UpdateAllPackages(install.Options{})
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(installer.options))
	assert.True(t, installer.options[0].APICacheEnabled)
	assert.Eq(t, 120, installer.options[0].APICacheTime)
	if installer.options[0].APICacheDir == "" {
		t.Fatalf("expected api cache dir to be derived from config, got %#v", installer.options[0])
	}
}

func TestUpdateAllPackagesUsesBatchConcurrencyAndPreservesResultOrder(t *testing.T) {
	block := make(chan struct{})
	installer := &fakeInstallService{block: block}
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
			cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
			cfg.Packages["fd"] = cfgpkg.Section{Repo: util.StringPtr("sharkdp/fd")}
			return cfg, nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{
				"junegunn/fzf":       {Repo: "junegunn/fzf", Tag: "v0.1.0"},
				"BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", Tag: "v0.1.0"},
				"sharkdp/fd":         {Repo: "sharkdp/fd", Tag: "v0.1.0"},
			}}, nil
		},
		LatestInfo: func(repo, _ string) (LatestInfo, error) {
			return LatestInfo{Tag: "v1.0.0"}, nil
		},
	}

	done := make(chan []UpdateResult, 1)
	errCh := make(chan error, 1)
	go func() {
		results, err := svc.UpdateAllPackages(install.Options{BatchConcurrency: 2})
		if err != nil {
			errCh <- err
			return
		}
		done <- results
	}()

	waitForMaxActive(t, func() int { return installer.currentMaxActive() }, 2)
	close(block)

	select {
	case err := <-errCh:
		t.Fatalf("update all packages: %v", err)
	case results := <-done:
		assert.Eq(t, []string{"fd", "fzf", "rg"}, []string{results[0].Name, results[1].Name, results[2].Name})
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for update all")
	}
	assert.Eq(t, 2, installer.currentMaxActive())
}

func TestListUpdateCandidatesPassesSourcePathToLatestChecker(t *testing.T) {
	svc := UpdateService{
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Packages["winmerge"] = cfgpkg.Section{
				Repo:       util.StringPtr("sourceforge:winmerge"),
				SourcePath: util.StringPtr("stable"),
			}
			return cfg, nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{
				"sourceforge:winmerge": {Repo: "sourceforge:winmerge", Tag: "2.16.42"},
			}}, nil
		},
		LatestInfo: func(repo, sourcePath string) (LatestInfo, error) {
			if repo != "sourceforge:winmerge" || sourcePath != "stable" {
				t.Fatalf("unexpected latest check repo=%q sourcePath=%q", repo, sourcePath)
			}
			return LatestInfo{Tag: "2.16.44"}, nil
		},
	}

	items, failures, checked, err := svc.ListUpdateCandidates()
	assert.NoErr(t, err)
	assert.Eq(t, 1, checked)
	assert.Eq(t, 0, len(failures))
	assert.Eq(t, 1, len(items))
	assert.Eq(t, "2.16.44", items[0].LatestTag)
}

func TestListUpdateCandidatesChecksForgeRepo(t *testing.T) {
	svc := UpdateService{
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Packages["fdroidserver"] = cfgpkg.Section{Repo: util.StringPtr("gitlab:gitlab.com/fdroid/fdroidserver")}
			return cfg, nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{
				"gitlab:gitlab.com/fdroid/fdroidserver": {Repo: "gitlab:gitlab.com/fdroid/fdroidserver", Tag: "v2.3.3"},
			}}, nil
		},
		LatestInfo: func(repo, sourcePath string) (LatestInfo, error) {
			if repo != "gitlab:gitlab.com/fdroid/fdroidserver" || sourcePath != "" {
				t.Fatalf("unexpected latest check repo=%q sourcePath=%q", repo, sourcePath)
			}
			return LatestInfo{Tag: "v2.3.4"}, nil
		},
	}

	items, failures, checked, err := svc.ListUpdateCandidates()

	assert.NoErr(t, err)
	assert.Eq(t, 1, checked)
	assert.Eq(t, 0, len(failures))
	assert.Eq(t, 1, len(items))
	assert.Eq(t, "v2.3.4", items[0].LatestTag)
}
