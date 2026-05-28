//go:build windows

package app

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestWindowsSelfReplaceScriptContainsQuotedPaths(t *testing.T) {
	script := windowsSelfReplaceScript(`C:\Tools\eget.exe`, `C:\Temp\eget-new.exe`, `C:\Tools\eget.exe.old`, `C:\Temp\eget-self-update.log`)

	assert.Contains(t, script, `"C:\Tools\eget.exe"`)
	assert.Contains(t, script, `"C:\Temp\eget-new.exe"`)
	assert.Contains(t, script, `"C:\Tools\eget.exe.old"`)
	assert.Contains(t, script, "move /Y")
}

func TestWindowsSelfReplaceScriptFastRetriesBeforeOneSecondSleep(t *testing.T) {
	script := windowsSelfReplaceScript(`C:\Tools\eget.exe`, `C:\Temp\eget-new.exe`, `C:\Tools\eget.exe.old`, `C:\Temp\eget-self-update.log`)

	assert.Contains(t, script, "set /A attempts+=1")
	assert.Contains(t, script, "EnableDelayedExpansion")
	assert.Contains(t, script, "if !attempts! GEQ 2000")
	assert.Contains(t, script, "timeout /T 1 /NOBREAK")
}

func TestWindowsSelfReplaceScriptWritesDiagnosticLog(t *testing.T) {
	script := windowsSelfReplaceScript(`C:\Tools\eget.exe`, `C:\Temp\eget-new.exe`, `C:\Tools\eget.exe.old`, `C:\Temp\eget-self-update.log`)

	assert.Contains(t, script, `"C:\Temp\eget-self-update.log"`)
	assert.Contains(t, script, "replace succeeded")
	assert.Contains(t, script, "restore backup")
	assert.Contains(t, script, ":replace")
	assert.Contains(t, script, "waiting to move replacement")
}
