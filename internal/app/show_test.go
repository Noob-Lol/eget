package app

import (
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	storepkg "github.com/inherelab/eget/internal/installed"
	"github.com/inherelab/eget/internal/util"
)

func TestShowPackageMergesConfiguredAndInstalledDetails(t *testing.T) {
	installedAt := time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC)
	releaseDate := time.Date(2026, 5, 16, 8, 0, 0, 0, time.UTC)
	svc := ShowService{
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			cfg.Packages["fzf"] = cfgpkg.Section{
				Repo:   util.StringPtr("junegunn/fzf"),
				Desc:   util.StringPtr("Manual fzf description"),
				Target: util.StringPtr("~/.local/bin"),
			}
			return cfg, nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{
				"junegunn/fzf": {
					Repo:           "junegunn/fzf",
					Target:         "junegunn/fzf",
					InstalledAt:    installedAt,
					URL:            "https://github.com/junegunn/fzf/releases/download/v0.50.0/fzf.tar.gz",
					Asset:          "fzf.tar.gz",
					ExtractedFiles: []string{"/usr/local/bin/fzf"},
					Tag:            "v0.50.0",
					Version:        "v0.50.0",
					ReleaseDate:    releaseDate,
					Desc:           "Repository fzf description",
					Homepage:       "https://junegunn.github.io/fzf",
					RepoURL:        "https://github.com/junegunn/fzf",
				},
			}}, nil
		},
	}

	got, err := svc.ShowPackage("fzf")
	if err != nil {
		t.Fatalf("ShowPackage: %v", err)
	}

	assert.Eq(t, "fzf", got.Name)
	assert.Eq(t, "junegunn/fzf", got.Repo)
	assert.Eq(t, "Manual fzf description", got.Desc)
	assert.Eq(t, "https://junegunn.github.io/fzf", got.Homepage)
	assert.Eq(t, "https://github.com/junegunn/fzf", got.RepoURL)
	assert.True(t, got.Configured)
	assert.True(t, got.Installed)
	assert.Eq(t, "v0.50.0", got.Version)
	assert.Eq(t, []string{"/usr/local/bin/fzf"}, got.ExtractedFiles)
}

func TestShowPackageSupportsInstalledOnlyPackageAndInfersHomepage(t *testing.T) {
	svc := ShowService{
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{
				"sourceforge:winmerge": {
					Repo:    "sourceforge:winmerge",
					Target:  "sourceforge:winmerge/stable",
					Version: "2.16.44",
					Desc:    "Windows visual diff and merge tool",
				},
			}}, nil
		},
	}

	got, err := svc.ShowPackage("sourceforge:winmerge")
	if err != nil {
		t.Fatalf("ShowPackage installed-only: %v", err)
	}

	assert.Eq(t, "winmerge", got.Name)
	assert.Eq(t, "sourceforge:winmerge", got.Repo)
	assert.Eq(t, "Windows visual diff and merge tool", got.Desc)
	assert.Eq(t, "https://sourceforge.net/projects/winmerge/", got.Homepage)
	assert.Eq(t, "https://sourceforge.net/projects/winmerge/", got.RepoURL)
	assert.False(t, got.Configured)
	assert.True(t, got.Installed)
	assert.Eq(t, "2.16.44", got.Version)
}

func TestShowPackageReturnsErrorForMissingTarget(t *testing.T) {
	svc := ShowService{
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return &storepkg.Config{Installed: map[string]storepkg.Entry{}}, nil
		},
	}

	_, err := svc.ShowPackage("missing")
	if err == nil {
		t.Fatal("expected missing package error")
	}
}
