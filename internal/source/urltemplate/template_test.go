package urltemplate

import (
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
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

func TestVariablesForUsesDefaultExecutableExt(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "windows exe", goos: "windows", want: ".exe"},
		{name: "linux empty", goos: "linux", want: ""},
		{name: "darwin empty", goos: "darwin", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars, err := VariablesFor(VariableInput{Name: "markview", Version: "1.2.3", GOOS: tt.goos, GOARCH: "amd64"})
			assert.NoErr(t, err)
			assert.Eq(t, tt.want, vars["ext"])
		})
	}
}

func TestVariablesForExtMapOverridesDefaultExecutableExt(t *testing.T) {
	vars, err := VariablesFor(VariableInput{
		Name:    "markview",
		Version: "1.2.3",
		GOOS:    "windows",
		GOARCH:  "amd64",
		Config: Config{
			ExtMap: map[string]string{"windows": ".zip"},
		},
	})
	assert.NoErr(t, err)
	assert.Eq(t, ".zip", vars["ext"])
}

func TestParseLatestTextAndJSON(t *testing.T) {
	got, err := ParseLatest([]byte("1.2.3\n"), Config{LatestFormat: "text"})
	assert.NoErr(t, err)
	assert.Eq(t, "1.2.3", got)

	got, err = ParseLatest([]byte(`{"version":"1.2.4"}`), Config{LatestFormat: "json", LatestJSONPath: "version"})
	assert.NoErr(t, err)
	assert.Eq(t, "1.2.4", got)
}

func TestParseLatestYAML(t *testing.T) {
	data := []byte("version: v1.2.5\nreleased_at: 2026-05-25T10:20:30+08:00\n")

	got, err := ParseLatest(data, Config{LatestFormat: "yaml"})
	assert.NoErr(t, err)
	assert.Eq(t, "v1.2.5", got)

	releasedAt, err := ParseLatestPublishedAt(data, Config{LatestFormat: "yaml"})
	assert.NoErr(t, err)
	if !releasedAt.Equal(time.Date(2026, 5, 25, 10, 20, 30, 0, time.FixedZone("", 8*60*60))) {
		t.Fatalf("expected released_at instant to match, got %s", releasedAt)
	}
}

func TestParseLatestYAMLPublishedAtWithoutTimezone(t *testing.T) {
	data := []byte("version: v1.2.5\nreleased_at: 2026-06-01T17:49:52\n")

	releasedAt, err := ParseLatestPublishedAt(data, Config{LatestFormat: "yaml"})

	assert.NoErr(t, err)
	assert.Eq(t, time.Date(2026, 6, 1, 17, 49, 52, 0, time.UTC), releasedAt)
}

func TestParseLatestYAMLDescription(t *testing.T) {
	data := []byte("version: v1.2.5\ndescription: Markdown preview tool\n")

	description, err := ParseLatestDescription(data, Config{LatestFormat: "yaml"})

	assert.NoErr(t, err)
	assert.Eq(t, "Markdown preview tool", description)
}

func TestParseLatestYAMLRequiresVersion(t *testing.T) {
	_, err := ParseLatest([]byte("released_at: 2026-05-25T10:20:30Z\n"), Config{LatestFormat: "yaml"})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), `yaml path "version" not found`)
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
