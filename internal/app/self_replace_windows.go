//go:build windows

package app

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

type DefaultExecutableReplacer struct{}

func (DefaultExecutableReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	backup := currentPath + ".old"
	logPath := replacementPath + ".replace.log"
	script := windowsSelfReplaceScript(currentPath, replacementPath, backup, logPath)
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
	cmd := exec.Command("cmd", "/C", scriptFile.Name())
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return SelfReplaceResult{}, err
	}
	return SelfReplaceResult{Deferred: true}, nil
}

func windowsSelfReplaceScript(currentPath, replacementPath, backupPath, logPath string) string {
	return fmt.Sprintf(`@echo off
setlocal
setlocal EnableDelayedExpansion
set attempts=0
echo start self replace %%DATE%% %%TIME%% > "%[4]s"
:wait
move /Y "%[1]s" "%[3]s" >nul 2>nul
if errorlevel 1 (
  set /A attempts+=1
  if !attempts! GEQ 2000 (
    echo waiting for current executable: "%[1]s" >> "%[4]s"
    set attempts=0
    timeout /T 1 /NOBREAK >nul
  )
  goto wait
)
set attempts=0
:replace
if not exist "%[2]s" (
  echo replacement missing, restore backup >> "%[4]s"
  move /Y "%[3]s" "%[1]s" >nul 2>nul
  exit /B 1
)
move /Y "%[2]s" "%[1]s" >nul 2>nul
if errorlevel 1 (
  set /A attempts+=1
  if !attempts! GEQ 2000 (
    echo waiting to move replacement: "%[2]s" >> "%[4]s"
    set attempts=0
    timeout /T 1 /NOBREAK >nul
  )
  goto replace
)
del /F /Q "%[3]s" >nul 2>nul
echo replace succeeded >> "%[4]s"
del /F /Q "%%~f0" >nul 2>nul
`, currentPath, replacementPath, backupPath, logPath)
}
