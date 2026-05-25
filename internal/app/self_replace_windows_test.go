//go:build windows

package app

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestWindowsSelfReplaceScriptContainsQuotedPaths(t *testing.T) {
	script := windowsSelfReplaceScript(`C:\Tools\eget.exe`, `C:\Temp\eget-new.exe`, `C:\Tools\eget.exe.old`)

	assert.Contains(t, script, `"C:\Tools\eget.exe"`)
	assert.Contains(t, script, `"C:\Temp\eget-new.exe"`)
	assert.Contains(t, script, `"C:\Tools\eget.exe.old"`)
	assert.Contains(t, script, "move /Y")
}
