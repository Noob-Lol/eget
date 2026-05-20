package urltemplate

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestRenderClaudeWindowsTemplate(t *testing.T) {
	cfg := Config{
		URLTemplate: "https://downloads.claude.ai/{version}/{os}-{arch}{libc}/claude{ext}",
		OSMap:       map[string]string{"windows": "win32", "linux": "linux", "darwin": "darwin"},
		ArchMap:     map[string]string{"amd64": "x64", "arm64": "arm64"},
		ExtMap:      map[string]string{"windows": ".exe", "linux": "", "darwin": ""},
		LibcMap:     map[string]string{"glibc": "", "musl": "-musl"},
	}
	vars, err := VariablesFor(VariableInput{Name: "claude", Version: "1.2.3", GOOS: "windows", GOARCH: "amd64", Config: cfg})
	assert.NoErr(t, err)
	got, err := Render(cfg.URLTemplate, vars)
	assert.NoErr(t, err)
	assert.Eq(t, "https://downloads.claude.ai/1.2.3/win32-x64/claude.exe", got)
}

func TestRenderClaudeLinuxMuslTemplate(t *testing.T) {
	cfg := Config{
		URLTemplate: "https://downloads.claude.ai/{version}/{os}-{arch}{libc}/claude{ext}",
		OSMap:       map[string]string{"linux": "linux"},
		ArchMap:     map[string]string{"amd64": "x64"},
		ExtMap:      map[string]string{"linux": ""},
		LibcMap:     map[string]string{"glibc": "", "musl": "-musl"},
	}
	vars, err := VariablesFor(VariableInput{Name: "claude", Version: "1.2.3", GOOS: "linux", GOARCH: "amd64", Libc: "musl", Config: cfg})
	assert.NoErr(t, err)
	got, err := Render(cfg.URLTemplate, vars)
	assert.NoErr(t, err)
	assert.Eq(t, "https://downloads.claude.ai/1.2.3/linux-x64-musl/claude", got)
}

func TestParseLatestTextAndJSON(t *testing.T) {
	got, err := ParseLatest([]byte("1.2.3\n"), Config{LatestFormat: "text"})
	assert.NoErr(t, err)
	assert.Eq(t, "1.2.3", got)

	got, err = ParseLatest([]byte(`{"version":"1.2.4"}`), Config{LatestFormat: "json", LatestJSONPath: "version"})
	assert.NoErr(t, err)
	assert.Eq(t, "1.2.4", got)
}

func TestExtractChecksumJSONPathWithRenderedPath(t *testing.T) {
	vars := map[string]string{"os": "linux", "arch": "x64", "libc": "-musl"}
	path, err := Render("platforms.{os}-{arch}{libc}.checksum", vars)
	assert.NoErr(t, err)
	got, err := ExtractJSONPath([]byte(`{"platforms":{"linux-x64-musl":{"checksum":"abc123"}}}`), path)
	assert.NoErr(t, err)
	assert.Eq(t, "abc123", got)
}

func TestRenderRejectsUnknownVariable(t *testing.T) {
	_, err := Render("https://example.com/{unknown}", map[string]string{"name": "tool"})
	if err == nil {
		t.Fatal("expected unknown variable error")
	}
}
