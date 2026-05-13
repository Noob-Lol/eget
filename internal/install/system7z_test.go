package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestResolveSystem7zPathUsesConfiguredPath(t *testing.T) {
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "custom-7z.exe")
	if err := os.WriteFile(exe, []byte("fake"), 0o755); err != nil {
		t.Fatalf("write fake 7z: %v", err)
	}

	got := resolveSystem7zPath(exe)
	assert.Eq(t, exe, got)
}

func TestResolveSystem7zPathFallsBackWhenConfiguredPathMissing(t *testing.T) {
	t.Setenv("PATH", "")

	got := resolveSystem7zPath(filepath.Join(t.TempDir(), "missing-7z.exe"))
	assert.Eq(t, "", got)
}

func TestShouldUseSystem7zForPreferredFormats(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		all      bool
		want     bool
	}{
		{name: "7z", filename: "tool.7z", want: true},
		{name: "rar", filename: "tool.rar", want: true},
		{name: "msi", filename: "setup.msi", want: true},
		{name: "cab", filename: "driver.cab", want: true},
		{name: "iso", filename: "image.iso", want: true},
		{name: "exe all", filename: "setup.exe", all: true, want: true},
		{name: "exe single", filename: "setup.exe", want: false},
		{name: "zip stays go", filename: "tool.zip", want: false},
		{name: "tar gz stays go", filename: "tool.tar.gz", want: false},
		{name: "tgz stays go", filename: "tool.tgz", want: false},
		{name: "tar xz stays go", filename: "tool.tar.xz", want: false},
		{name: "txz stays go", filename: "tool.txz", want: false},
		{name: "tar bz2 stays go", filename: "tool.tar.bz2", want: false},
		{name: "tbz stays go", filename: "tool.tbz", want: false},
		{name: "tar zst stays go", filename: "tool.tar.zst", want: false},
		{name: "single gz stays go", filename: "tool.gz", want: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Eq(t, tt.want, shouldUseSystem7z(tt.filename, tt.all))
		})
	}
}
