package install

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestBinaryChooserSkipsMetadataFiles(t *testing.T) {
	chooser := NewBinaryChooser("bd")

	t.Run("keeps extensionless binary candidate", func(t *testing.T) {
		direct, possible := chooser.Choose("bd", false, 0)
		assert.True(t, direct)
		assert.True(t, possible)
	})

	t.Run("skips extensionless metadata files", func(t *testing.T) {
		for _, name := range []string{"LICENSE", "README", "CHANGELOG", "NOTICE"} {
			direct, possible := chooser.Choose(name, false, 0)
			assert.False(t, direct)
			assert.False(t, possible)
		}
	})
}

func TestFileChooserSupportsExcludePatterns(t *testing.T) {
	chooser, err := NewFileChooser("*.exe,^*x86*,^*.sig")
	assert.NoErr(t, err)

	tests := []struct {
		name string
		want bool
	}{
		{name: "bin/tool-win64.exe", want: true},
		{name: "bin/tool-x86.exe", want: false},
		{name: "bin/tool-win64.exe.sig", want: false},
		{name: "docs/readme.txt", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			direct, possible := chooser.Choose(tt.name, false, 0)
			assert.False(t, direct)
			assert.Eq(t, tt.want, possible)
		})
	}
}

func TestFileChooserExcludeOnlyDefaultsToAllFiles(t *testing.T) {
	chooser, err := NewFileChooser("^*x86*,^*.sig")
	assert.NoErr(t, err)

	tests := []struct {
		name string
		want bool
	}{
		{name: "bin/tool-win64.exe", want: true},
		{name: "bin/tool-x86.exe", want: false},
		{name: "bin/tool-win64.exe.sig", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			direct, possible := chooser.Choose(tt.name, false, 0)
			assert.False(t, direct)
			assert.Eq(t, tt.want, possible)
		})
	}
}
