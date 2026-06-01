package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	"github.com/inherelab/eget/internal/app"
	cfgpkg "github.com/inherelab/eget/internal/config"
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
