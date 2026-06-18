package app

import (
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/util"
)

func TestAddPackage(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	svc := ConfigService{
		ConfigPath: configPath,
		Load: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
		Save: cfgpkg.Save,
		RepoMetadata: func(repo string) (RepoMetadata, error) {
			assert.Eq(t, "junegunn/fzf", repo)
			return RepoMetadata{Desc: "Command-line fuzzy finder"}, nil
		},
	}

	opts := install.Options{
		Output:          "~/.local/bin",
		CacheDir:        "~/.cache/eget",
		System:          "linux/amd64",
		ExtractFile:     "fzf",
		Asset:           []string{"linux_amd64"},
		Tag:             "nightly",
		Verify:          "sha256:123",
		Source:          true,
		SourcePath:      "stable",
		DisableSSL:      true,
		All:             true,
		StripComponents: 1,
		IsGUI:           true,
		RenameFiles:     map[string]string{"fzf-linux-amd64": "fzf"},
	}

	if err := svc.AddPackage("junegunn/fzf", "", opts); err != nil {
		t.Fatalf("add package: %v", err)
	}

	cfg, err := cfgpkg.LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	pkg, ok := cfg.Packages["fzf"]
	if !ok {
		t.Fatal("expected packages.fzf to be created")
	}
	if pkg.Repo == nil || *pkg.Repo != "junegunn/fzf" {
		t.Fatalf("expected repo to be persisted, got %#v", pkg.Repo)
	}
	if pkg.Target == nil || *pkg.Target != "~/.local/bin" {
		t.Fatalf("expected target to be persisted, got %#v", pkg.Target)
	}
	if pkg.CacheDir == nil || *pkg.CacheDir != "~/.cache/eget" {
		t.Fatalf("expected cache_dir to be persisted, got %#v", pkg.CacheDir)
	}
	if pkg.Source == nil || !*pkg.Source {
		t.Fatalf("expected download_source to be persisted, got %#v", pkg.Source)
	}
	if pkg.SourcePath == nil || *pkg.SourcePath != "stable" {
		t.Fatalf("expected source_path to be persisted, got %#v", pkg.SourcePath)
	}
	if pkg.DisableSSL == nil || !*pkg.DisableSSL {
		t.Fatalf("expected disable_ssl to be persisted, got %#v", pkg.DisableSSL)
	}
	if len(pkg.AssetFilters) != 1 || pkg.AssetFilters[0] != "linux_amd64" {
		t.Fatalf("expected asset filters to be persisted, got %#v", pkg.AssetFilters)
	}
	if pkg.IsGUI == nil || !*pkg.IsGUI {
		t.Fatalf("expected is_gui to be persisted, got %#v", pkg.IsGUI)
	}
	if pkg.StripComponents == nil || *pkg.StripComponents != 1 {
		t.Fatalf("expected strip_components to be persisted, got %#v", pkg.StripComponents)
	}
	assert.Eq(t, map[string]string{"fzf-linux-amd64": "fzf"}, pkg.RenameFiles)
	assert.Eq(t, "Command-line fuzzy finder", *pkg.Desc)
}

func TestAddPackageWithCustomName(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	svc := ConfigService{
		ConfigPath: configPath,
		Load: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
		Save: cfgpkg.Save,
	}

	if err := svc.AddPackage("junegunn/fzf", "myfzf", install.Options{}); err != nil {
		t.Fatalf("add package: %v", err)
	}

	cfg, err := cfgpkg.LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if _, ok := cfg.Packages["myfzf"]; !ok {
		t.Fatal("expected packages.myfzf to be created")
	}
}

func TestAddPackageWritesPkgTemplateReference(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	cfg := cfgpkg.NewFile()
	cfg.PkgTemplates["mydev"] = cfgpkg.Section{
		LatestURL:   util.StringPtr("http://mydev.lan/tools/{name}/latest.yaml"),
		URLTemplate: util.StringPtr("http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}"),
	}

	svc := ConfigService{
		ConfigPath: configPath,
		Load: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
		Save: cfgpkg.Save,
	}

	assert.NoErr(t, svc.AddPackage("mydev:markview", "", install.Options{}))

	loaded, err := cfgpkg.LoadFile(configPath)
	assert.NoErr(t, err)
	pkg, ok := loaded.Packages["markview"]
	assert.True(t, ok)
	assert.Eq(t, "pkg-template:mydev:markview", *pkg.Repo)
	assert.Eq(t, "markview", *pkg.Name)
}

func TestAddPackagePersistsChunkConcurrency(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	svc := ConfigService{
		ConfigPath: configPath,
		Load: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
		Save: cfgpkg.Save,
	}

	err := svc.AddPackage("sharkdp/fd", "fd", install.Options{ChunkConcurrency: 3, ChunkConcurrencySet: true})
	assert.NoErr(t, err)

	cfg, err := cfgpkg.LoadFile(configPath)
	assert.NoErr(t, err)
	assert.Eq(t, 3, *cfg.Packages["fd"].ChunkConcurrency)
}

func TestAddPackageNormalizesSourceForgeTargetWithPath(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	svc := ConfigService{
		ConfigPath: configPath,
		Load: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
		Save: cfgpkg.Save,
	}

	if err := svc.AddPackage("sourceforge:winmerge/stable", "", install.Options{}); err != nil {
		t.Fatalf("add package: %v", err)
	}

	cfg, err := cfgpkg.LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	pkg, ok := cfg.Packages["winmerge"]
	if !ok {
		t.Fatal("expected packages.winmerge to be created")
	}
	if pkg.Repo == nil || *pkg.Repo != "sourceforge:winmerge" {
		t.Fatalf("expected repo to be normalized, got %#v", pkg.Repo)
	}
	if pkg.SourcePath == nil || *pkg.SourcePath != "stable" {
		t.Fatalf("expected source_path to be persisted, got %#v", pkg.SourcePath)
	}
}

func TestAddPackageNormalizesForgeTargets(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		wantName string
		wantRepo string
	}{
		{name: "gitlab default host", repo: "gitlab:fdroid/fdroidserver", wantName: "fdroidserver", wantRepo: "gitlab:gitlab.com/fdroid/fdroidserver"},
		{name: "gitea explicit host", repo: "gitea:codeberg.org/forgejo/forgejo", wantName: "forgejo", wantRepo: "gitea:codeberg.org/forgejo/forgejo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			configPath := filepath.Join(tmp, "eget.toml")
			svc := ConfigService{
				ConfigPath: configPath,
				Load: func() (*cfgpkg.File, error) {
					return cfgpkg.NewFile(), nil
				},
				Save: cfgpkg.Save,
			}

			if err := svc.AddPackage(tt.repo, "", install.Options{}); err != nil {
				t.Fatalf("add forge package: %v", err)
			}

			cfg, err := cfgpkg.LoadFile(configPath)
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			pkg, ok := cfg.Packages[tt.wantName]
			if !ok {
				t.Fatalf("expected packages.%s, got %#v", tt.wantName, cfg.Packages)
			}
			if pkg.Repo == nil || *pkg.Repo != tt.wantRepo {
				t.Fatalf("expected normalized repo %q, got %#v", tt.wantRepo, pkg.Repo)
			}
		})
	}
}
