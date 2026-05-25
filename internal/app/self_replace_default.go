//go:build !windows

package app

import "os"

type DefaultExecutableReplacer struct{}

func (DefaultExecutableReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	info, err := os.Stat(currentPath)
	if err != nil {
		return SelfReplaceResult{}, err
	}
	if err := os.Chmod(replacementPath, info.Mode()); err != nil {
		return SelfReplaceResult{}, err
	}

	backup := currentPath + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(currentPath, backup); err != nil {
		return SelfReplaceResult{}, err
	}
	if err := os.Rename(replacementPath, currentPath); err != nil {
		_ = os.Rename(backup, currentPath)
		return SelfReplaceResult{}, err
	}
	_ = os.Remove(backup)
	return SelfReplaceResult{}, nil
}
