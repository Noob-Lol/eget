package app

import (
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	storepkg "github.com/inherelab/eget/internal/installed"
	"github.com/inherelab/eget/internal/util"
)

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
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			repo := target.Repo
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
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			repo := target.Repo
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
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			repo := target.Repo
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
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
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

func TestUpdateAllPackagesNoProxySkipsConfiguredProxyURL(t *testing.T) {
	t.Setenv("NO_PROXY", "")
	installer := &fakeInstallService{}
	proxyURL := "http://127.0.0.1:7890"
	svc := UpdateService{
		Install: installer,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Global.ProxyURL = &proxyURL
			cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
			return cfg, nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{
				"BurntSushi/ripgrep": {Repo: "BurntSushi/ripgrep", Tag: "v13.0.0"},
			}}, nil
		},
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			return LatestInfo{Tag: "v14.0.0"}, nil
		},
	}

	_, err := svc.UpdateAllPackages(install.Options{NoProxy: true})

	assert.NoErr(t, err)
	assert.Eq(t, 1, len(installer.options))
	assert.Eq(t, "", installer.options[0].ProxyURL)
	assert.True(t, installer.options[0].NoProxy)
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
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
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
