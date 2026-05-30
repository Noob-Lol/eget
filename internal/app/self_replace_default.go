//go:build !windows

package app

import (
	"errors"
	"syscall"
)

type DefaultExecutableReplacer struct{}

func (DefaultExecutableReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	ops := defaultSelfReplaceFileOps()
	ops.IsCrossDeviceRename = isCrossDeviceRename
	return replaceExecutable(currentPath, replacementPath, ops)
}

func isCrossDeviceRename(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
