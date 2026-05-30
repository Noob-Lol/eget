package app

import (
	"io"
	"os"
	"path/filepath"
)

type selfReplaceFileOps struct {
	Rename              func(oldpath, newpath string) error
	IsCrossDeviceRename func(error) bool
}

func defaultSelfReplaceFileOps() selfReplaceFileOps {
	return selfReplaceFileOps{
		Rename:              os.Rename,
		IsCrossDeviceRename: func(error) bool { return false },
	}
}

func replaceExecutable(currentPath, replacementPath string, ops selfReplaceFileOps) (SelfReplaceResult, error) {
	if ops.Rename == nil {
		ops.Rename = os.Rename
	}
	if ops.IsCrossDeviceRename == nil {
		ops.IsCrossDeviceRename = func(error) bool { return false }
	}

	info, err := os.Stat(currentPath)
	if err != nil {
		return SelfReplaceResult{}, err
	}
	mode := info.Mode()
	if err := os.Chmod(replacementPath, mode); err != nil {
		return SelfReplaceResult{}, err
	}

	backup := currentPath + ".old"
	_ = os.Remove(backup)
	if err := ops.Rename(currentPath, backup); err != nil {
		return SelfReplaceResult{}, err
	}
	if err := ops.Rename(replacementPath, currentPath); err != nil {
		if ops.IsCrossDeviceRename(err) {
			if copyErr := copyReplacementIntoPlace(replacementPath, currentPath, mode, ops); copyErr == nil {
				_ = os.Remove(replacementPath)
				_ = os.Remove(backup)
				return SelfReplaceResult{}, nil
			} else {
				_ = ops.Rename(backup, currentPath)
				return SelfReplaceResult{}, copyErr
			}
		}
		_ = ops.Rename(backup, currentPath)
		return SelfReplaceResult{}, err
	}
	_ = os.Remove(backup)
	return SelfReplaceResult{}, nil
}

func copyReplacementIntoPlace(replacementPath, currentPath string, mode os.FileMode, ops selfReplaceFileOps) error {
	in, err := os.Open(replacementPath)
	if err != nil {
		return err
	}
	defer in.Close()

	dir := filepath.Dir(currentPath)
	base := filepath.Base(currentPath)
	tmp, err := os.CreateTemp(dir, "."+base+".new-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ops.Rename(tmpPath, currentPath); err != nil {
		return err
	}
	removeTmp = false
	return nil
}
