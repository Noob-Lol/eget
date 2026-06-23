package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestReplaceExecutableFallsBackWhenReplacementRenameCrossesDevice(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "eget")
	replacement := filepath.Join(t.TempDir(), "eget")
	assert.NoErr(t, os.WriteFile(current, []byte("old"), 0o755))
	assert.NoErr(t, os.WriteFile(replacement, []byte("new"), 0o644))
	currentInfo, err := os.Stat(current)
	assert.NoErr(t, err)

	errCrossDevice := errors.New("invalid cross-device link")
	ops := defaultSelfReplaceFileOps()
	ops.Rename = func(oldpath, newpath string) error {
		if oldpath == replacement && newpath == current {
			return errCrossDevice
		}
		return os.Rename(oldpath, newpath)
	}
	ops.IsCrossDeviceRename = func(err error) bool {
		return errors.Is(err, errCrossDevice)
	}

	_, err = replaceExecutable(current, replacement, ops)

	assert.NoErr(t, err)
	body, err := os.ReadFile(current)
	assert.NoErr(t, err)
	assert.Eq(t, "new", string(body))
	info, err := os.Stat(current)
	assert.NoErr(t, err)
	assert.Eq(t, currentInfo.Mode().Perm(), info.Mode().Perm())
	_, err = os.Stat(current + ".old")
	assert.True(t, os.IsNotExist(err))
}
