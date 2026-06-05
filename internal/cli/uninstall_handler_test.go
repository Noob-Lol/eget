package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	"github.com/inherelab/eget/internal/app"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	storepkg "github.com/inherelab/eget/internal/installed"
)

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

func TestHandleUninstallPrintsGUIInstallerManualHint(t *testing.T) {
	store := &fakeUninstallStoreForCLI{
		cfg: &storepkg.Config{Installed: map[string]storepkg.Entry{
			"ansxuman/Clauge": {
				Repo:        "ansxuman/Clauge",
				IsGUI:       true,
				InstallMode: install.InstallModeInstaller,
			},
		}},
	}
	svc := &cliService{
		uninstallService: app.UninstallService{
			Store: store,
			LoadConfig: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				cfg.Packages["Clauge"] = cfgpkg.Section{Repo: strPtrForUninstallTest("ansxuman/Clauge")}
				return cfg, nil
			},
		},
	}
	opened := false
	origGOOS := uninstallRuntimeGOOS
	origOpen := openWindowsProgramsAndFeatures
	uninstallRuntimeGOOS = "windows"
	openWindowsProgramsAndFeatures = func() error {
		opened = true
		return nil
	}
	defer func() {
		uninstallRuntimeGOOS = origGOOS
		openWindowsProgramsAndFeatures = origOpen
	}()

	out := captureUninstallStdout(t, func() {
		err := svc.handle("uninstall", &UninstallOptions{Target: "Clauge", Yes: true})
		assert.NoErr(t, err)
	})

	assert.True(t, opened)
	assert.Contains(t, out, "removed_files: 0")
	assert.Contains(t, out, "manual_uninstall_required")
	assert.Contains(t, out, "opened_uninstall_settings: Windows Programs and Features")
}

func captureUninstallStdout(t *testing.T, run func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	assert.NoErr(t, err)
	defer r.Close()
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	run()

	assert.NoErr(t, w.Close())
	var out bytes.Buffer
	_, err = io.Copy(&out, r)
	assert.NoErr(t, err)
	return out.String()
}

func strPtrForUninstallTest(value string) *string {
	return &value
}

func TestHandleUninstallDoesNotPrintGUIInstallerHintForPortable(t *testing.T) {
	store := &fakeUninstallStoreForCLI{
		cfg: &storepkg.Config{Installed: map[string]storepkg.Entry{
			"sipeed/picoclaw": {
				Repo:        "sipeed/picoclaw",
				IsGUI:       true,
				InstallMode: install.InstallModePortable,
			},
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
	origGOOS := uninstallRuntimeGOOS
	origOpen := openWindowsProgramsAndFeatures
	uninstallRuntimeGOOS = "windows"
	openWindowsProgramsAndFeatures = func() error {
		t.Fatal("portable GUI uninstall should not open Windows uninstall settings")
		return nil
	}
	defer func() {
		uninstallRuntimeGOOS = origGOOS
		openWindowsProgramsAndFeatures = origOpen
	}()

	out := captureUninstallStdout(t, func() {
		err := svc.handle("uninstall", &UninstallOptions{Target: "sipeed/picoclaw", Yes: true})
		assert.NoErr(t, err)
	})

	if strings.Contains(out, "manual_uninstall_required") {
		t.Fatalf("expected no manual uninstall hint for portable GUI app, got %q", out)
	}
}
