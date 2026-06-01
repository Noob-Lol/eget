package app

import (
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
)

func TestInstallTargetUsesGuiTargetForPortableGUI(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://github.com/sipeed/picoclaw/releases/download/v0.2.7/picoclaw.exe",
			Asset:          "picoclaw.exe",
			ExtractedFiles: []string{"picoclaw.exe"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			target := "~/bin"
			guiTarget := "~/Applications"
			repo := "sipeed/picoclaw"
			isGUI := true
			cfg.Global.Target = &target
			cfg.Global.GuiTarget = &guiTarget
			cfg.Packages["picoclaw"] = cfgpkg.Section{Repo: &repo, IsGUI: &isGUI}
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("picoclaw", install.Options{})
	if err != nil {
		t.Fatalf("install gui package: %v", err)
	}
	if !runner.opts.IsGUI {
		t.Fatalf("expected IsGUI option, got %#v", runner.opts)
	}
	if runner.opts.GuiTarget == "" || !strings.Contains(runner.opts.GuiTarget, "Applications") {
		t.Fatalf("expected expanded GuiTarget, got %#v", runner.opts.GuiTarget)
	}
	if runner.opts.OutputExplicit {
		t.Fatalf("expected OutputExplicit false without --to")
	}
}

func TestInstallTargetKeepsExplicitOutputForGUI(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:            "https://github.com/sipeed/picoclaw/releases/download/v0.2.7/picoclaw.exe",
			Asset:          "picoclaw.exe",
			ExtractedFiles: []string{"D:/Apps/PicoClaw/picoclaw.exe"},
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			target := "~/bin"
			guiTarget := "~/Applications"
			cfg.Global.Target = &target
			cfg.Global.GuiTarget = &guiTarget
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("sipeed/picoclaw", install.Options{IsGUI: true, Output: "D:/Apps/PicoClaw"})
	if err != nil {
		t.Fatalf("install gui package with output: %v", err)
	}
	if !runner.opts.OutputExplicit {
		t.Fatalf("expected OutputExplicit true when --to is provided")
	}
	if runner.opts.Output != "D:/Apps/PicoClaw" {
		t.Fatalf("expected explicit output to win, got %#v", runner.opts.Output)
	}
}

func TestInstallTargetPassesConfiguredInstallMode(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:           "https://github.com/Syngnat/GoNavi/releases/download/v0.7.7/GoNavi-0.7.7-Windows-Amd64.exe",
			Asset:         "GoNavi-0.7.7-Windows-Amd64.exe",
			IsGUI:         true,
			InstallMode:   install.InstallModeInstaller,
			InstallerFile: "C:/Temp/GoNavi-0.7.7-Windows-Amd64.exe",
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			cfg := cfgpkg.NewFile()
			repo := "Syngnat/GoNavi"
			name := "gonavi"
			isGUI := true
			installMode := install.InstallModeInstaller
			cfg.Packages["gonavi"] = cfgpkg.Section{
				Repo:        &repo,
				Name:        &name,
				IsGUI:       &isGUI,
				InstallMode: &installMode,
			}
			return cfg, nil
		},
	}

	_, err := svc.InstallTarget("gonavi", install.Options{})
	assert.NoErr(t, err)
	assert.True(t, runner.opts.IsGUI)
	assert.Eq(t, install.InstallModeInstaller, runner.opts.InstallMode)
}

func TestInstallTargetPassesCLIInstallMode(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:           "https://github.com/Syngnat/GoNavi/releases/download/v0.7.7/GoNavi-0.7.7-Windows-Amd64.exe",
			Asset:         "GoNavi-0.7.7-Windows-Amd64.exe",
			IsGUI:         true,
			InstallMode:   install.InstallModeInstaller,
			InstallerFile: "C:/Temp/GoNavi-0.7.7-Windows-Amd64.exe",
		},
	}
	svc := Service{
		Runner: runner,
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
	}

	_, err := svc.InstallTarget("Syngnat/GoNavi", install.Options{IsGUI: true, InstallMode: install.InstallModeInstaller})
	assert.NoErr(t, err)
	assert.True(t, runner.opts.IsGUI)
	assert.Eq(t, install.InstallModeInstaller, runner.opts.InstallMode)
}

func TestInstallTargetRecordsGUIInstallerWithoutExtractedFiles(t *testing.T) {
	runner := &fakeRunner{
		result: RunResult{
			URL:           "https://github.com/sipeed/picoclaw/releases/download/v0.2.7/picoclaw-setup.exe",
			Asset:         "picoclaw-setup.exe",
			IsGUI:         true,
			InstallMode:   install.InstallModeInstaller,
			InstallerFile: "C:/Temp/picoclaw-setup.exe",
		},
	}
	store := &fakeInstalledStore{}
	svc := Service{
		Runner: runner,
		Store:  store,
		Now:    func() time.Time { return time.Unix(1710000000, 0).UTC() },
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfgpkg.NewFile(), nil
		},
	}

	_, err := svc.InstallTarget("sipeed/picoclaw", install.Options{IsGUI: true})
	if err != nil {
		t.Fatalf("install gui installer: %v", err)
	}
	if store.calls != 1 {
		t.Fatalf("expected installer install to be recorded, got %d calls", store.calls)
	}
	if !store.entry.IsGUI || store.entry.InstallMode != install.InstallModeInstaller {
		t.Fatalf("expected gui installer metadata, got %#v", store.entry)
	}
	if len(store.entry.ExtractedFiles) != 0 {
		t.Fatalf("expected no extracted files for installer, got %#v", store.entry.ExtractedFiles)
	}
}
