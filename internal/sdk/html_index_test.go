package sdk

import (
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestParseHTMLIndexForGoLinks(t *testing.T) {
	body := strings.NewReader(`
<html><body>
<a href="go1.21.1.windows-amd64.zip">go1.21.1.windows-amd64.zip</a>
<a href="go1.21.1.linux-amd64.tar.gz">go1.21.1.linux-amd64.tar.gz</a>
<a href="README.txt">README</a>
</body></html>
`)
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)

	index, err := ParseHTMLIndex(body, HTMLParseOptions{
		SDK:       "go",
		SourceURL: "https://mirrors.example.com/golang/",
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("parse html index: %v", err)
	}

	assert.Eq(t, 1, index.Schema)
	assert.Eq(t, "go", index.SDK)
	assert.Eq(t, now, index.FetchedAt)
	assert.Eq(t, 1, len(index.Items))
	item := index.Items[0]
	assert.Eq(t, "1.21.1", item.Version)
	assert.True(t, item.Stable)
	assert.Eq(t, 2, len(item.Files))
	windowsFile := indexFileByOS(t, item.Files, "windows")
	assert.Eq(t, "amd64", windowsFile.Arch)
	assert.Eq(t, "zip", windowsFile.Ext)
	assert.Eq(t, "https://mirrors.example.com/golang/go1.21.1.windows-amd64.zip", windowsFile.URL)
	linuxFile := indexFileByOS(t, item.Files, "linux")
	assert.Eq(t, "tar.gz", linuxFile.Ext)
}

func TestParseHTMLIndexForNodeLinksWithPathPrefix(t *testing.T) {
	body := strings.NewReader(`
<a href="/other/node-v20.11.1-win-x64.zip">ignore</a>
<a href="/binaries/node/v20.11.1/node-v20.11.1-win-x64.zip">node</a>
<a href="/binaries/node/v20.11.1/node-v20.11.1-linux-x64.tar.xz">node</a>
`)

	index, err := ParseHTMLIndex(body, HTMLParseOptions{
		SDK:             "node",
		SourceURL:       "https://registry.npmmirror.com/binary.html",
		IndexPathPrefix: "/binaries/node/",
	})
	if err != nil {
		t.Fatalf("parse html index: %v", err)
	}

	assert.Eq(t, 1, len(index.Items))
	assert.Eq(t, "20.11.1", index.Items[0].Version)
	assert.Eq(t, 2, len(index.Items[0].Files))
	winFile := indexFileByOS(t, index.Items[0].Files, "win")
	assert.Eq(t, "x64", winFile.Arch)
	assert.Eq(t, "https://registry.npmmirror.com/binaries/node/v20.11.1/node-v20.11.1-win-x64.zip", winFile.URL)
}

func TestParseHTMLIndexUsesCustomFilenamePattern(t *testing.T) {
	body := strings.NewReader(`<a href="zig-0.12.0-windows-x86_64.zip">zig</a>`)

	index, err := ParseHTMLIndex(body, HTMLParseOptions{
		SDK:             "zig",
		SourceURL:       "https://example.com/zig/",
		FilenamePattern: "zig-{version}-{os}-{arch}.{ext}",
	})
	if err != nil {
		t.Fatalf("parse html index: %v", err)
	}

	assert.Eq(t, "0.12.0", index.Items[0].Version)
	assert.Eq(t, "windows", index.Items[0].Files[0].OS)
	assert.Eq(t, "x86_64", index.Items[0].Files[0].Arch)
	assert.Eq(t, "zip", index.Items[0].Files[0].Ext)
}

func TestParseHTMLIndexErrorsWhenNoFilesMatch(t *testing.T) {
	_, err := ParseHTMLIndex(strings.NewReader(`<a href="README.txt">README</a>`), HTMLParseOptions{
		SDK:       "go",
		SourceURL: "https://example.com/",
	})
	assert.Err(t, err)
}

func indexFileByOS(t *testing.T, files []IndexFile, osName string) IndexFile {
	t.Helper()
	for _, file := range files {
		if file.OS == osName {
			return file
		}
	}
	t.Fatalf("expected file for os %q in %#v", osName, files)
	return IndexFile{}
}
