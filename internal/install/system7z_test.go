package install

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseSystem7zListOutput(t *testing.T) {
	output := `
Path = tool.7z
Type = 7z

----------
Path = bin/tool.exe
Size = 12
Packed Size = 8
Modified = 2026-05-13 10:00:00
Attributes = A
CRC = 12345678
Encrypted = -
Method = LZMA2
Block = 0

Path = docs/
Folder = +
Size = 0
Packed Size = 0
Attributes = D
`

	files, err := parseSystem7zListOutput([]byte(output))
	if err != nil {
		t.Fatalf("parse 7z list output: %v", err)
	}

	assert.Eq(t, 2, len(files))
	assert.Eq(t, "bin"+string(os.PathSeparator)+"tool.exe", files[0].Name)
	assert.False(t, files[0].Dir())
	assert.Eq(t, "docs", files[1].Name)
	assert.True(t, files[1].Dir())
}

func TestParseSystem7zListOutputRejectsUnsafePath(t *testing.T) {
	output := `
Path = evil.7z
Type = 7z

----------
Path = ../evil.exe
Size = 1
`

	_, err := parseSystem7zListOutput([]byte(output))
	if err == nil {
		t.Fatal("expected unsafe archive path error")
	}
	assert.Contains(t, err.Error(), "unsafe archive path")
}

func TestSystem7zExtractorSelectsCandidateAndExtractsFile(t *testing.T) {
	tmp := t.TempDir()
	var extractArgs []string
	origRunner := runSystem7zCommand
	defer func() { runSystem7zCommand = origRunner }()

	runSystem7zCommand = func(exe string, args ...string) ([]byte, error) {
		if args[0] == "l" {
			return []byte(`
Path = tool.7z
Type = 7z

----------
Path = bin/tool.exe
Size = 4
`), nil
		}
		extractArgs = append([]string(nil), args...)
		outDir := ""
		member := args[len(args)-1]
		for _, arg := range args {
			if strings.HasPrefix(arg, "-o") {
				outDir = strings.TrimPrefix(arg, "-o")
			}
		}
		if outDir == "" {
			t.Fatal("expected output dir")
		}
		extracted := filepath.Join(outDir, filepath.FromSlash(member))
		if err := os.MkdirAll(filepath.Dir(extracted), 0o755); err != nil {
			return nil, err
		}
		return nil, os.WriteFile(extracted, []byte("tool"), 0o755)
	}

	extractor := NewSystem7zExtractor("tool.7z", "tool", NewBinaryChooser("tool"), "7z")
	file, candidates, err := extractor.Extract([]byte("archive"), false)
	if err != nil {
		t.Fatalf("extract candidate: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected direct selected file, got candidates %#v", candidates)
	}

	out := filepath.Join(tmp, "tool.exe")
	if err := file.Extract(out); err != nil {
		t.Fatalf("extract selected file: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	assert.Eq(t, "tool", string(data))
	assert.Contains(t, strings.Join(extractArgs, " "), "bin/tool.exe")
}
