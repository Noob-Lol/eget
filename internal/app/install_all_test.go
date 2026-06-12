package app

import (
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/util"
)

func TestInstallAllPackagesInstallsConfiguredPackagesByName(t *testing.T) {
	runner := &fakeBatchRunner{
		results: map[string]RunResult{
			"junegunn/fzf": {
				URL:            "https://github.com/junegunn/fzf/releases/download/v0.50.0/fzf.tar.gz",
				Tool:           "fzf",
				ExtractedFiles: []string{"./fzf"},
			},
			"BurntSushi/ripgrep": {
				URL:            "https://github.com/BurntSushi/ripgrep/releases/download/v14.0.0/rg.tar.gz",
				Tool:           "rg",
				ExtractedFiles: []string{"./rg"},
			},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Packages["fzf"] = cfgpkg.Section{
				Repo:         util.StringPtr("junegunn/fzf"),
				AssetFilters: []string{"linux"},
			}
			cfg.Packages["rg"] = cfgpkg.Section{
				Repo: util.StringPtr("BurntSushi/ripgrep"),
				File: util.StringPtr("rg"),
			}
			return cfg, nil
		},
	}

	results, err := svc.InstallAllPackages(install.Options{Quiet: true})
	if err != nil {
		t.Fatalf("install all packages: %v", err)
	}

	assert.Eq(t, []string{"BurntSushi/ripgrep", "junegunn/fzf"}, sortedStrings(runner.targets))
	assert.Eq(t, 2, len(results))
	assert.Eq(t, "fzf", results[0].Name)
	assert.Eq(t, "junegunn/fzf", results[0].Target)
	assert.Eq(t, "rg", results[1].Name)
	assert.Eq(t, "BurntSushi/ripgrep", results[1].Target)
	assert.True(t, runner.opts["junegunn/fzf"].Quiet)
	assert.Eq(t, []string{"linux"}, runner.opts["junegunn/fzf"].Asset)
	assert.Eq(t, "rg", runner.opts["BurntSushi/ripgrep"].ExtractFile)
}

func sortedStrings(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

func TestInstallAllPackagesRejectsEmptyConfig(t *testing.T) {
	svc := Service{
		Runner: &fakeBatchRunner{},
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
	}

	_, err := svc.InstallAllPackages(install.Options{})
	if err == nil {
		t.Fatal("expected empty managed package config to fail")
	}
	if !strings.Contains(err.Error(), "no managed packages") {
		t.Fatalf("expected no managed packages error, got %v", err)
	}
}

type fakeBatchRunner struct {
	mu        sync.Mutex
	targets   []string
	opts      map[string]install.Options
	results   map[string]RunResult
	active    int
	maxActive int
	block     chan struct{}
}

func TestInstallAllPackagesUsesBatchConcurrencyAndPreservesResultOrder(t *testing.T) {
	block := make(chan struct{})
	runner := &fakeBatchRunner{
		block: block,
		results: map[string]RunResult{
			"junegunn/fzf":       {Tool: "fzf"},
			"BurntSushi/ripgrep": {Tool: "rg"},
			"sharkdp/fd":         {Tool: "fd"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
			cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
			cfg.Packages["fd"] = cfgpkg.Section{Repo: util.StringPtr("sharkdp/fd")}
			return cfg, nil
		},
	}

	done := make(chan []InstallAllResult, 1)
	errCh := make(chan error, 1)
	go func() {
		results, err := svc.InstallAllPackages(install.Options{BatchConcurrency: 2, Quiet: true})
		if err != nil {
			errCh <- err
			return
		}
		done <- results
	}()

	waitForMaxActive(t, func() int { return runner.currentMaxActive() }, 2)
	close(block)

	select {
	case err := <-errCh:
		t.Fatalf("install all packages: %v", err)
	case results := <-done:
		assert.Eq(t, []string{"fd", "fzf", "rg"}, []string{results[0].Name, results[1].Name, results[2].Name})
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for install all")
	}
	assert.Eq(t, 2, runner.currentMaxActive())
}

func TestInstallAllPackagesUsesAutoBatchConcurrency(t *testing.T) {
	block := make(chan struct{})
	runner := &fakeBatchRunner{
		block: block,
		results: map[string]RunResult{
			"junegunn/fzf":       {Tool: "fzf"},
			"BurntSushi/ripgrep": {Tool: "rg"},
			"sharkdp/fd":         {Tool: "fd"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Packages["fzf"] = cfgpkg.Section{Repo: util.StringPtr("junegunn/fzf")}
			cfg.Packages["rg"] = cfgpkg.Section{Repo: util.StringPtr("BurntSushi/ripgrep")}
			cfg.Packages["fd"] = cfgpkg.Section{Repo: util.StringPtr("sharkdp/fd")}
			return cfg, nil
		},
	}

	done := make(chan []InstallAllResult, 1)
	errCh := make(chan error, 1)
	go func() {
		results, err := svc.InstallAllPackages(install.Options{BatchConcurrency: 0, BatchConcurrencySet: true, Quiet: true})
		if err != nil {
			errCh <- err
			return
		}
		done <- results
	}()

	waitForMaxActive(t, func() int { return runner.currentMaxActive() }, 3)
	close(block)

	select {
	case err := <-errCh:
		t.Fatalf("install all packages: %v", err)
	case results := <-done:
		assert.Eq(t, []string{"fd", "fzf", "rg"}, []string{results[0].Name, results[1].Name, results[2].Name})
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for install all")
	}
	assert.Eq(t, 3, runner.currentMaxActive())
}
