package install

import (
	"regexp"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestSelectDetector(t *testing.T) {
	svc := NewService()
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: "all"}
	}
	svc.SystemDetectorFactory = func(goos, goarch string) (Detector, error) {
		return &fakeDetector{name: goos + "/" + goarch}, nil
	}
	svc.AssetDetectorFactory = func(asset string, anti bool, re *regexp.Regexp) Detector {
		name := asset
		if re != nil {
			name = "REG:" + asset
		}
		if anti {
			name = "^" + name
		}
		return &fakeDetector{name: name}
	}
	svc.DetectorChainFactory = func(detectors []Detector, system Detector) Detector {
		return &fakeDetector{name: "chain"}
	}

	detector, err := svc.SelectDetector(&Options{System: "all"})
	if err != nil {
		t.Fatalf("SelectDetector(all): %v", err)
	}
	if got := detector.(*fakeDetector).name; got != "all" {
		t.Fatalf("SelectDetector(all) = %q, want %q", got, "all")
	}

	detector, err = svc.SelectDetector(&Options{System: "linux/amd64", Asset: []string{"cli", "^arm"}})
	if err != nil {
		t.Fatalf("SelectDetector(custom): %v", err)
	}
	if got := detector.(*fakeDetector).name; got != "chain" {
		t.Fatalf("SelectDetector(custom) = %q, want %q", got, "chain")
	}

	detector, err = svc.SelectDetector(&Options{System: "linux/amd64", Asset: []string{`REG:\.deb$`, `^REG:\.sha256$`}})
	if err != nil {
		t.Fatalf("SelectDetector(regex): %v", err)
	}
	if got := detector.(*fakeDetector).name; got != "chain" {
		t.Fatalf("SelectDetector(regex) = %q, want %q", got, "chain")
	}
}

func TestSelectDetectorAppliesPlatformAssetFiltersForTargetSystem(t *testing.T) {
	svc := NewService()
	var gotAssets []string
	svc.SystemDetectorFactory = func(goos, goarch string) (Detector, error) {
		return &fakeDetector{name: goos + "/" + goarch}, nil
	}
	svc.AssetDetectorFactory = func(asset string, anti bool, re *regexp.Regexp) Detector {
		name := asset
		if re != nil {
			name = "REG:" + asset
		}
		if anti {
			name = "^" + name
		}
		gotAssets = append(gotAssets, name)
		return &fakeDetector{name: name}
	}
	svc.DetectorChainFactory = func(detectors []Detector, system Detector) Detector {
		return &fakeDetector{name: "chain"}
	}

	_, err := svc.SelectDetector(&Options{
		System: "windows/amd64",
		Asset:  []string{"windows:zip", "linux:tar.gz", "windows:^REG:\\.sha256$", "plain"},
	})
	if err != nil {
		t.Fatalf("SelectDetector(platform filters): %v", err)
	}
	assert.Eq(t, []string{"zip", "^REG:\\.sha256$", "plain"}, gotAssets)
}

func TestSelectDetectorReturnsSystemDetectorWhenPlatformAssetFiltersAreSkipped(t *testing.T) {
	svc := NewService()
	svc.SystemDetectorFactory = func(goos, goarch string) (Detector, error) {
		return &fakeDetector{name: goos + "/" + goarch}, nil
	}
	svc.AssetDetectorFactory = func(asset string, anti bool, re *regexp.Regexp) Detector {
		t.Fatalf("unexpected asset detector for skipped filter %q", asset)
		return nil
	}
	svc.DetectorChainFactory = func(detectors []Detector, system Detector) Detector {
		t.Fatalf("unexpected detector chain for skipped filters")
		return nil
	}

	detector, err := svc.SelectDetector(&Options{
		System: "darwin/arm64",
		Asset:  []string{"windows:zip", "linux:tar.gz"},
	})
	if err != nil {
		t.Fatalf("SelectDetector(skipped platform filters): %v", err)
	}
	assert.Eq(t, "darwin/arm64", detector.(*fakeDetector).name)
}

func TestAssetFilterForGOOSKeepsUnknownPrefixAsPlainFilter(t *testing.T) {
	expr, ok := assetFilterForGOOS("foo:bar", "linux")
	if !ok {
		t.Fatal("expected unknown prefix filter to remain enabled")
	}
	assert.Eq(t, "foo:bar", expr)
}

func TestParseAssetFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantExpr string
		wantAnti bool
		wantRe   bool
	}{
		{name: "plain include", input: "deb", wantExpr: "deb"},
		{name: "plain exclude", input: "^deb", wantExpr: "deb", wantAnti: true},
		{name: "regex include", input: `REG:\.deb$`, wantExpr: `\.deb$`, wantRe: true},
		{name: "regex exclude", input: `^REG:\.deb$`, wantExpr: `\.deb$`, wantAnti: true, wantRe: true},
		{name: "prefix include", input: `PRE:codex`, wantExpr: `codex`, wantRe: true},
		{name: "prefix exclude", input: `^PRE:codex`, wantExpr: `codex`, wantAnti: true, wantRe: true},
		{name: "suffix include", input: `SUF:.zip`, wantExpr: `.zip`, wantRe: true},
		{name: "suffix exclude", input: `^SUF:.sha256`, wantExpr: `.sha256`, wantAnti: true, wantRe: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAssetFilter(tt.input)
			if err != nil {
				t.Fatalf("parseAssetFilter(%q): %v", tt.input, err)
			}
			if got.Expr != tt.wantExpr || got.Anti != tt.wantAnti || (got.Regex != nil) != tt.wantRe {
				t.Fatalf("parseAssetFilter(%q) = %#v", tt.input, got)
			}
		})
	}
}

func TestParseAssetFilterRejectsBadRegex(t *testing.T) {
	_, err := parseAssetFilter(`REG:[abc`)
	if err == nil {
		t.Fatal("expected invalid regex to fail")
	}
}
