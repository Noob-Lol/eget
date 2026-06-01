package install

import (
	"testing"
)

func TestAssetDetectorSupportsRegexInclude(t *testing.T) {
	re, err := compileAssetRegex(`\.deb$`)
	if err != nil {
		t.Fatalf("compileAssetRegex: %v", err)
	}
	d := &assetDetector{Asset: `\.deb$`, Regex: re}

	got, candidates, err := d.Detect([]string{
		"https://example.com/pkg_1.0.0_amd64.deb",
		"https://example.com/pkg_1.0.0_amd64.rpm",
	})
	if err != nil {
		t.Fatalf("Detect(): %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", candidates)
	}
	if got != "https://example.com/pkg_1.0.0_amd64.deb" {
		t.Fatalf("expected deb asset to match, got %q", got)
	}
}

func TestAssetDetectorSupportsRegexExclude(t *testing.T) {
	re, err := compileAssetRegex(`\.deb$`)
	if err != nil {
		t.Fatalf("compileAssetRegex: %v", err)
	}
	d := &assetDetector{Asset: `\.deb$`, Anti: true, Regex: re}

	got, candidates, err := d.Detect([]string{
		"https://example.com/pkg_1.0.0_amd64.deb",
		"https://example.com/pkg_1.0.0_amd64.rpm",
	})
	if err != nil {
		t.Fatalf("Detect(): %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", candidates)
	}
	if got != "https://example.com/pkg_1.0.0_amd64.rpm" {
		t.Fatalf("expected rpm asset to remain after exclude, got %q", got)
	}
}

func TestAssetDetectorSupportsPrefixInclude(t *testing.T) {
	filter, err := parseAssetFilter(`PRE:codex-app`)
	if err != nil {
		t.Fatalf("parseAssetFilter: %v", err)
	}
	d := &assetDetector{Asset: filter.Expr, Regex: filter.Regex}

	got, candidates, err := d.Detect([]string{
		"https://example.com/codex-app-server-x86_64-pc-windows-msvc.zip",
		"https://example.com/codex-command-runner-x86_64-pc-windows-msvc.zip",
	})
	if err != nil {
		t.Fatalf("Detect(): %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", candidates)
	}
	if got != "https://example.com/codex-app-server-x86_64-pc-windows-msvc.zip" {
		t.Fatalf("expected prefix asset to match, got %q", got)
	}
}

func TestAssetDetectorSupportsSuffixExclude(t *testing.T) {
	filter, err := parseAssetFilter(`^SUF:.sha256`)
	if err != nil {
		t.Fatalf("parseAssetFilter: %v", err)
	}
	d := &assetDetector{Asset: filter.Expr, Anti: filter.Anti, Regex: filter.Regex}

	got, candidates, err := d.Detect([]string{
		"https://example.com/codex.exe",
		"https://example.com/codex.exe.sha256",
	})
	if err != nil {
		t.Fatalf("Detect(): %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", candidates)
	}
	if got != "https://example.com/codex.exe" {
		t.Fatalf("expected sha256 asset to be excluded, got %q", got)
	}
}

func TestAssetDetectorMatchesPlainFilterCaseInsensitive(t *testing.T) {
	d := &assetDetector{Asset: "setup"}

	got, candidates, err := d.Detect([]string{
		"https://example.com/WinMerge-2.16.56-x64-Setup.exe",
		"https://example.com/WinMerge-2.16.56-x64.zip",
	})
	if err != nil {
		t.Fatalf("Detect(): %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", candidates)
	}
	if got != "https://example.com/WinMerge-2.16.56-x64-Setup.exe" {
		t.Fatalf("expected setup asset to match, got %q", got)
	}
}
