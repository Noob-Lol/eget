//go:build windows

package app

import (
	"fmt"
	"os"
	"os/exec"
)

type DefaultExecutableReplacer struct{}

func (DefaultExecutableReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	backup := currentPath + ".old"
	script := windowsSelfReplaceScript(currentPath, replacementPath, backup)
	scriptFile, err := os.CreateTemp("", "eget-self-update-*.cmd")
	if err != nil {
		return SelfReplaceResult{}, err
	}
	if _, err := scriptFile.WriteString(script); err != nil {
		_ = scriptFile.Close()
		return SelfReplaceResult{}, err
	}
	if err := scriptFile.Close(); err != nil {
		return SelfReplaceResult{}, err
	}
	cmd := exec.Command("cmd", "/C", "start", "", "/B", scriptFile.Name())
	if err := cmd.Start(); err != nil {
		return SelfReplaceResult{}, err
	}
	return SelfReplaceResult{Deferred: true}, nil
}

func windowsSelfReplaceScript(currentPath, replacementPath, backupPath string) string {
	return fmt.Sprintf(`@echo off
setlocal
:wait
move /Y "%[1]s" "%[3]s" >nul 2>nul
if errorlevel 1 (
  timeout /T 1 /NOBREAK >nul
  goto wait
)
move /Y "%[2]s" "%[1]s" >nul 2>nul
if errorlevel 1 (
  move /Y "%[3]s" "%[1]s" >nul 2>nul
  exit /B 1
)
del /F /Q "%[3]s" >nul 2>nul
del /F /Q "%%~f0" >nul 2>nul
`, currentPath, replacementPath, backupPath)
}
