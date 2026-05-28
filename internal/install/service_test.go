package install

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
	forge "github.com/inherelab/eget/internal/source/forge"
	sourcegithub "github.com/inherelab/eget/internal/source/github"
	sourcesf "github.com/inherelab/eget/internal/source/sourceforge"
	"github.com/inherelab/eget/internal/source/urltemplate"
)

type fakeDetector struct {
	name string
}

func (f *fakeDetector) Detect(assets []string) (string, []string, error) {
	return f.name, nil, nil
}

type fakeVerifier struct {
	name string
}

func (f *fakeVerifier) Verify(b []byte) error {
	return nil
}

type fakeChooser struct {
	name string
}

type chooserRecorder struct {
	value any
}

type fakeExtractor struct {
	name string
}

func (f *fakeExtractor) Extract([]byte, bool) (ExtractedFile, []ExtractedFile, error) {
	return ExtractedFile{}, nil, nil
}

type fakeHTTPGetterFunc func(url string) (*http.Response, error)

func (f fakeHTTPGetterFunc) Get(url string) (*http.Response, error) {
	return f(url)
}

func TestNewDefaultServiceWiring(t *testing.T) {
	svc := NewDefaultService(
		fakeHTTPGetterFunc(func(url string) (*http.Response, error) {
			if url == "https://example.com/tool.sha256" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")),
				}, nil
			}
			return nil, errors.New("unexpected url")
		}),
		func(tool, output string) time.Time { return time.Unix(123, 0) },
	)

	finder, tool, err := svc.SelectFinder("inhere/markview", &Options{UpgradeOnly: true})
	if err != nil {
		t.Fatalf("SelectFinder(default): %v", err)
	}
	if tool != "markview" {
		t.Fatalf("tool = %q, want %q", tool, "markview")
	}
	if _, ok := finder.(*sourcegithub.AssetFinder); !ok {
		t.Fatalf("finder type = %T, want *github.AssetFinder", finder)
	}

	detector, err := svc.SelectDetector(&Options{System: "linux/amd64", Asset: []string{"cli"}})
	if err != nil {
		t.Fatalf("SelectDetector(default): %v", err)
	}
	if detector == nil {
		t.Fatal("expected detector")
	}

	verifier, err := svc.SelectVerifier("https://example.com/tool.sha256", &Options{})
	if err != nil {
		t.Fatalf("SelectVerifier(default): %v", err)
	}
	if err := verifier.Verify([]byte("test")); err != nil {
		t.Fatalf("Verify(default): %v", err)
	}

	extractor, err := SelectExtractorAs[Extractor](svc, "https://example.com/tool.tar.gz", "tool", &Options{})
	if err != nil {
		t.Fatalf("SelectExtractor(default): %v", err)
	}
	if extractor == nil {
		t.Fatal("expected extractor")
	}
}

func TestSelectFinder(t *testing.T) {
	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "tool.tar.gz")
	if err := os.WriteFile(localFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	svc := NewService()
	svc.GitHubGetter = fakeHTTPGetterFunc(func(url string) (*http.Response, error) {
		return nil, nil
	})
	svc.GitHubGetterFactory = func(opts Options) sourcegithub.HTTPGetter {
		return fakeHTTPGetterFunc(func(url string) (*http.Response, error) {
			if opts.ProxyURL != "http://127.0.0.1:7890" {
				t.Fatalf("expected proxy url to propagate to finder getter, got %q", opts.ProxyURL)
			}
			return nil, nil
		})
	}
	svc.BinaryModTime = func(tool, output string) time.Time {
		return time.Unix(123, 0)
	}

	t.Run("repo target", func(t *testing.T) {
		opts := &Options{Tag: "v1.2.3", ProxyURL: "http://127.0.0.1:7890"}
		finder, tool, err := svc.SelectFinder("inhere/markview", opts)
		if err != nil {
			t.Fatalf("SelectFinder(repo): %v", err)
		}
		if tool != "markview" {
			t.Fatalf("tool = %q, want %q", tool, "markview")
		}
		got, ok := finder.(*sourcegithub.AssetFinder)
		if !ok {
			t.Fatalf("finder type = %T, want *github.AssetFinder", finder)
		}
		if got.Repo != "inhere/markview" || got.Tag != "tags/v1.2.3" {
			t.Fatalf("finder = %+v", got)
		}
	})

	t.Run("github url", func(t *testing.T) {
		opts := &Options{Source: true, Tag: "main"}
		finder, tool, err := svc.SelectFinder("https://github.com/inhere/markview", opts)
		if err != nil {
			t.Fatalf("SelectFinder(github url): %v", err)
		}
		if tool != "markview" {
			t.Fatalf("tool = %q, want %q", tool, "markview")
		}
		got, ok := finder.(*sourcegithub.SourceFinder)
		if !ok {
			t.Fatalf("finder type = %T, want *github.SourceFinder", finder)
		}
		if got.Repo != "inhere/markview" || got.Tag != "main" || got.Tool != "markview" {
			t.Fatalf("finder = %+v", got)
		}
	})

	t.Run("direct url", func(t *testing.T) {
		opts := &Options{}
		finder, tool, err := svc.SelectFinder("https://example.com/tool.tar.gz", opts)
		if err != nil {
			t.Fatalf("SelectFinder(direct): %v", err)
		}
		if tool != "" {
			t.Fatalf("tool = %q, want empty", tool)
		}
		got, ok := finder.(*DirectAssetFinder)
		if !ok {
			t.Fatalf("finder type = %T, want *DirectAssetFinder", finder)
		}
		if got.URL != "https://example.com/tool.tar.gz" {
			t.Fatalf("URL = %q", got.URL)
		}
		if opts.System != "all" {
			t.Fatalf("opts.System = %q, want %q", opts.System, "all")
		}
	})

	t.Run("direct url preserves explicit system", func(t *testing.T) {
		opts := &Options{System: "windows/amd64"}
		_, _, err := svc.SelectFinder("https://example.com/tool.zip", opts)
		if err != nil {
			t.Fatalf("SelectFinder(direct explicit system): %v", err)
		}
		if opts.System != "windows/amd64" {
			t.Fatalf("opts.System = %q, want %q", opts.System, "windows/amd64")
		}
	})

	t.Run("local file", func(t *testing.T) {
		opts := &Options{}
		finder, tool, err := svc.SelectFinder(localFile, opts)
		if err != nil {
			t.Fatalf("SelectFinder(local): %v", err)
		}
		if tool != "" {
			t.Fatalf("tool = %q, want empty", tool)
		}
		got, ok := finder.(*DirectAssetFinder)
		if !ok {
			t.Fatalf("finder type = %T, want *DirectAssetFinder", finder)
		}
		if got.URL != localFile {
			t.Fatalf("URL = %q, want %q", got.URL, localFile)
		}
		if opts.System != "all" {
			t.Fatalf("opts.System = %q, want %q", opts.System, "all")
		}
	})

	t.Run("local file preserves explicit system", func(t *testing.T) {
		opts := &Options{System: "windows/amd64"}
		_, _, err := svc.SelectFinder(localFile, opts)
		if err != nil {
			t.Fatalf("SelectFinder(local explicit system): %v", err)
		}
		if opts.System != "windows/amd64" {
			t.Fatalf("opts.System = %q, want %q", opts.System, "windows/amd64")
		}
	})

	t.Run("sourceforge target", func(t *testing.T) {
		opts := &Options{SourcePath: "stable", Tag: "2.16.44", ProxyURL: "http://127.0.0.1:7890"}
		svc.SourceForgeGetterFactory = func(opts Options) sourcesf.HTTPGetter {
			return fakeHTTPGetterFunc(func(url string) (*http.Response, error) {
				if opts.ProxyURL != "http://127.0.0.1:7890" {
					t.Fatalf("expected proxy url to propagate to sourceforge getter, got %q", opts.ProxyURL)
				}
				return nil, nil
			})
		}

		finder, tool, err := svc.SelectFinder("sourceforge:winmerge", opts)
		if err != nil {
			t.Fatalf("SelectFinder(sourceforge): %v", err)
		}
		if tool != "winmerge" {
			t.Fatalf("tool = %q, want %q", tool, "winmerge")
		}
		got, ok := finder.(sourcesf.Finder)
		if !ok {
			t.Fatalf("finder type = %T, want sourceforge.Finder", finder)
		}
		if got.Project != "winmerge" || got.Path != "stable" || got.Tag != "2.16.44" {
			t.Fatalf("finder = %+v", got)
		}
		if got.Getter == nil {
			t.Fatal("expected sourceforge getter")
		}
	})

	t.Run("sourceforge target path", func(t *testing.T) {
		svc.SourceForgeGetterFactory = func(opts Options) sourcesf.HTTPGetter {
			return fakeHTTPGetterFunc(func(url string) (*http.Response, error) { return nil, nil })
		}

		finder, tool, err := svc.SelectFinder("sourceforge:winmerge/stable", &Options{})
		if err != nil {
			t.Fatalf("SelectFinder(sourceforge path): %v", err)
		}
		if tool != "winmerge" {
			t.Fatalf("tool = %q, want %q", tool, "winmerge")
		}
		got, ok := finder.(sourcesf.Finder)
		if !ok {
			t.Fatalf("finder type = %T, want sourceforge.Finder", finder)
		}
		if got.Path != "stable" {
			t.Fatalf("finder path = %q, want stable", got.Path)
		}
	})

	t.Run("sourceforge conflicting paths", func(t *testing.T) {
		_, _, err := svc.SelectFinder("sourceforge:winmerge/beta", &Options{SourcePath: "stable"})
		if err == nil || !strings.Contains(err.Error(), "source_path") {
			t.Fatalf("expected source_path conflict, got %v", err)
		}
	})

	t.Run("forge gitlab target", func(t *testing.T) {
		opts := &Options{Tag: "v1.2.3", ProxyURL: "http://127.0.0.1:7890"}
		var gotProxyURL string
		svc.ForgeGetterFactory = func(opts Options) forge.HTTPGetter {
			gotProxyURL = opts.ProxyURL
			return fakeHTTPGetterFunc(func(url string) (*http.Response, error) { return nil, nil })
		}

		finder, tool, err := svc.SelectFinder("gitlab:fdroid/fdroidserver", opts)
		if err != nil {
			t.Fatalf("SelectFinder(gitlab): %v", err)
		}
		if tool != "fdroidserver" {
			t.Fatalf("tool = %q, want fdroidserver", tool)
		}
		got, ok := finder.(forge.Finder)
		if !ok {
			t.Fatalf("finder type = %T, want forge.Finder", finder)
		}
		if got.Target.Normalized != "gitlab:gitlab.com/fdroid/fdroidserver" || got.Tag != "v1.2.3" || got.Getter == nil {
			t.Fatalf("unexpected forge finder: %+v", got)
		}
		if gotProxyURL != "http://127.0.0.1:7890" {
			t.Fatalf("expected proxy url to propagate to forge getter, got %q", gotProxyURL)
		}
	})

	t.Run("forge gitea target", func(t *testing.T) {
		opts := &Options{Tag: "v9.0.0"}
		svc.ForgeGetterFactory = func(opts Options) forge.HTTPGetter {
			return fakeHTTPGetterFunc(func(url string) (*http.Response, error) { return nil, nil })
		}

		finder, tool, err := svc.SelectFinder("gitea:codeberg.org/forgejo/forgejo", opts)
		if err != nil {
			t.Fatalf("SelectFinder(gitea): %v", err)
		}
		if tool != "forgejo" {
			t.Fatalf("tool = %q, want forgejo", tool)
		}
		got, ok := finder.(forge.Finder)
		if !ok || got.Target.Provider != forge.ProviderGitea {
			t.Fatalf("finder type = %T value=%+v, want gitea forge.Finder", finder, got)
		}
		if got.Target.Normalized != "gitea:codeberg.org/forgejo/forgejo" || got.Tag != "v9.0.0" || got.Getter == nil {
			t.Fatalf("unexpected forge finder: %+v", got)
		}
	})

	t.Run("forge target without getter factory", func(t *testing.T) {
		svc.ForgeGetterFactory = nil
		_, _, err := svc.SelectFinder("gitlab:fdroid/fdroidserver", &Options{})
		if err == nil || !strings.Contains(err.Error(), "forge getter factory is required") {
			t.Fatalf("expected forge getter factory error, got %v", err)
		}
	})

	t.Run("template target", func(t *testing.T) {
		var requests []string
		svc.TemplateGetterFactory = func(opts Options) urltemplate.HTTPGetter {
			if opts.ProxyURL != "http://127.0.0.1:7890" {
				t.Fatalf("expected proxy url to propagate to template getter, got %q", opts.ProxyURL)
			}
			return fakeHTTPGetterFunc(func(url string) (*http.Response, error) {
				requests = append(requests, url)
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("1.2.3")),
				}, nil
			})
		}

		finder, tool, err := svc.SelectFinder("template:claude", &Options{
			ProxyURL: "http://127.0.0.1:7890",
			System:   "windows/amd64",
			URLTemplate: URLTemplateOptions{
				LatestURL:   "https://example.com/latest",
				URLTemplate: "https://example.com/{version}/{os}-{arch}/claude{ext}",
				OSMap:       map[string]string{"windows": "win32"},
				ArchMap:     map[string]string{"amd64": "x64"},
				ExtMap:      map[string]string{"windows": ".exe"},
			},
		})
		if err != nil {
			t.Fatalf("SelectFinder(template): %v", err)
		}
		if tool != "claude" {
			t.Fatalf("tool = %q, want claude", tool)
		}
		got, ok := finder.(*urltemplate.Finder)
		if !ok {
			t.Fatalf("finder type = %T, want *urltemplate.Finder", finder)
		}
		assets, err := got.Find()
		if err != nil {
			t.Fatalf("Find(template): %v", err)
		}
		assert.Eq(t, []string{"https://example.com/1.2.3/win32-x64/claude.exe"}, assets)
		assert.Eq(t, []string{"https://example.com/latest"}, requests)
	})

	t.Run("invalid target", func(t *testing.T) {
		if _, _, err := svc.SelectFinder("invalid-target", &Options{}); err == nil {
			t.Fatal("expected invalid target error")
		}
	})
}

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

func TestSelectVerifier(t *testing.T) {
	svc := NewService()
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		if expected == "bad" {
			return nil, errors.New("bad verifier")
		}
		return &fakeVerifier{name: "verify:" + expected}, nil
	}
	svc.Sha256AssetVerifierFactory = func(assetURL string, opts Options) Verifier {
		_ = opts
		return &fakeVerifier{name: "asset:" + assetURL}
	}
	svc.Sha256PrinterFactory = func() Verifier {
		return &fakeVerifier{name: "printer"}
	}
	svc.NoVerifierFactory = func() Verifier {
		return &fakeVerifier{name: "noop"}
	}

	verifier, err := svc.SelectVerifier("sum.txt", &Options{Verify: "abc"})
	if err != nil {
		t.Fatalf("SelectVerifier(verify): %v", err)
	}
	if got := verifier.(*fakeVerifier).name; got != "verify:abc" {
		t.Fatalf("SelectVerifier(verify) = %q", got)
	}

	verifier, err = svc.SelectVerifier("sum.txt", &Options{})
	if err != nil {
		t.Fatalf("SelectVerifier(asset): %v", err)
	}
	if got := verifier.(*fakeVerifier).name; got != "asset:sum.txt" {
		t.Fatalf("SelectVerifier(asset) = %q", got)
	}

	verifier, err = svc.SelectVerifier("", &Options{Hash: true})
	if err != nil {
		t.Fatalf("SelectVerifier(hash): %v", err)
	}
	if got := verifier.(*fakeVerifier).name; got != "printer" {
		t.Fatalf("SelectVerifier(hash) = %q", got)
	}

	verifier, err = svc.SelectVerifier("", &Options{})
	if err != nil {
		t.Fatalf("SelectVerifier(noop): %v", err)
	}
	if got := verifier.(*fakeVerifier).name; got != "noop" {
		t.Fatalf("SelectVerifier(noop) = %q", got)
	}
}

func TestTemplateChecksumVerifierUsesRenderedManifest(t *testing.T) {
	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()

	var requestedURL string
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		_ = client
		requestedURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"platforms":{"win32-x64":{"checksum":"abc"}}}`)),
		}, nil
	}

	svc := NewService()
	var verifierValue string
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		verifierValue = expected
		return &fakeVerifier{name: "verify:" + expected}, nil
	}

	verifier, err := svc.SelectVerifier("", &Options{
		URLTemplate: URLTemplateOptions{
			ChecksumURLTemplate: "https://example.com/{version}/manifest.json",
			ChecksumFormat:      "json",
			ChecksumJSONPath:    "platforms.{os}-{arch}.checksum",
			ResolvedVars:        map[string]string{"version": "1.2.3", "os": "win32", "arch": "x64"},
		},
	})
	if err != nil {
		t.Fatalf("SelectVerifier(template checksum): %v", err)
	}

	assert.Eq(t, "verify:abc", verifier.(*fakeVerifier).name)
	assert.Eq(t, "abc", verifierValue)
	assert.Eq(t, "https://example.com/1.2.3/manifest.json", requestedURL)
}

func TestTemplateChecksumVerifierPrefersExplicitVerify(t *testing.T) {
	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()

	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		t.Fatalf("manifest should not be requested when verify_sha256 is set")
		return nil, nil
	}

	svc := NewService()
	var verifierValue string
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		verifierValue = expected
		return &fakeVerifier{name: "verify:" + expected}, nil
	}

	verifier, err := svc.SelectVerifier("", &Options{
		Verify: "explicit",
		URLTemplate: URLTemplateOptions{
			ChecksumURLTemplate: "https://example.com/{version}/manifest.json",
			ChecksumFormat:      "json",
			ChecksumJSONPath:    "platforms.{os}-{arch}.checksum",
			ResolvedVars:        map[string]string{"version": "1.2.3", "os": "win32", "arch": "x64"},
		},
	})
	if err != nil {
		t.Fatalf("SelectVerifier(explicit verify): %v", err)
	}

	assert.Eq(t, "verify:explicit", verifier.(*fakeVerifier).name)
	assert.Eq(t, "explicit", verifierValue)
}

func TestSelectExtractor(t *testing.T) {
	svc := NewService()
	svc.DownloadOnlyExtractorFactory = func(name string) any {
		return &fakeExtractor{name: "download:" + name}
	}
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		if pattern == "bad[" {
			return nil, errors.New("bad glob")
		}
		return &fakeChooser{name: "glob:" + pattern}, nil
	}
	svc.BinaryChooserFactory = func(tool string) any {
		return &fakeChooser{name: "binary:" + tool}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		ch := chooser.(*fakeChooser)
		return &fakeExtractor{name: filename + "|" + tool + "|" + ch.name}
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.tar.gz", "tool", &Options{DownloadOnly: true})
	if err != nil {
		t.Fatalf("SelectExtractor(download): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "download:tool.tar.gz" {
		t.Fatalf("SelectExtractor(download) = %q", got)
	}

	extractor, err = svc.SelectExtractor("https://example.com/tool.tar.gz", "tool", &Options{ExtractFile: "LICENSE"})
	if err != nil {
		t.Fatalf("SelectExtractor(glob): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "tool.tar.gz|tool|glob:LICENSE" {
		t.Fatalf("SelectExtractor(glob) = %q", got)
	}

	extractor, err = svc.SelectExtractor("https://example.com/tool.tar.gz", "tool", &Options{})
	if err != nil {
		t.Fatalf("SelectExtractor(binary): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "tool.tar.gz|tool|binary:tool" {
		t.Fatalf("SelectExtractor(binary) = %q", got)
	}
}

func TestSelectExtractorUsesSystem7zForSevenZipWhenAvailable(t *testing.T) {
	svc := NewService()
	svc.BinaryChooserFactory = func(tool string) any {
		return NewBinaryChooser(tool)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return &fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		if configured != "C:/Tools/7z.exe" {
			t.Fatalf("expected configured 7z path to propagate, got %q", configured)
		}
		return "C:/Tools/7z.exe"
	}
	svc.System7zExtractorFactory = func(filename, tool string, chooser Chooser, exe string) Extractor {
		return &fakeExtractor{name: "system7z:" + filepath.Base(filename)}
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.7z", "tool", &Options{Sys7zPath: "C:/Tools/7z.exe"})
	if err != nil {
		t.Fatalf("SelectExtractor(system 7z): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "system7z:tool.7z" {
		t.Fatalf("expected system 7z extractor, got %q", got)
	}
}

func TestSelectExtractorFallsBackToGoExtractorWithoutSystem7z(t *testing.T) {
	svc := NewService()
	svc.BinaryChooserFactory = func(tool string) any {
		return NewBinaryChooser(tool)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return &fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		return ""
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.7z", "tool", &Options{})
	if err != nil {
		t.Fatalf("SelectExtractor(go fallback): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "go:tool.7z" {
		t.Fatalf("expected Go extractor fallback, got %q", got)
	}
}

func TestSelectExtractorKeepsTarGzOnGoExtractorEvenWithSystem7z(t *testing.T) {
	svc := NewService()
	svc.BinaryChooserFactory = func(tool string) any {
		return NewBinaryChooser(tool)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return &fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		return "C:/Tools/7z.exe"
	}
	svc.System7zExtractorFactory = func(filename, tool string, chooser Chooser, exe string) Extractor {
		return &fakeExtractor{name: "system7z:" + filepath.Base(filename)}
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.tar.gz", "tool", &Options{})
	if err != nil {
		t.Fatalf("SelectExtractor(tar.gz): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "go:tool.tar.gz" {
		t.Fatalf("expected tar.gz to stay on Go extractor, got %q", got)
	}
}

func TestSelectExtractorDoesNotUseSystem7zForPureDownloadOnly(t *testing.T) {
	svc := NewService()
	svc.DownloadOnlyExtractorFactory = func(name string) any {
		return &fakeExtractor{name: "download:" + name}
	}
	svc.System7zPathResolver = func(configured string) string {
		return "C:/Tools/7z.exe"
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.7z", "tool", &Options{DownloadOnly: true})
	if err != nil {
		t.Fatalf("SelectExtractor(download-only): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "download:tool.7z" {
		t.Fatalf("expected pure download-only extractor, got %q", got)
	}
}

func TestSelectExtractorTreatsDownloadWithExtractFileAsArchiveExtraction(t *testing.T) {
	svc := NewService()
	rec := &chooserRecorder{}
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return &fakeChooser{name: "glob:" + pattern}, nil
	}
	svc.BinaryChooserFactory = func(tool string) any {
		return &fakeChooser{name: "binary:" + tool}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		rec.value = chooser
		ch := chooser.(*fakeChooser)
		return &fakeExtractor{name: filename + "|" + tool + "|" + ch.name}
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.tar.gz", "tool", &Options{
		DownloadOnly: true,
		ExtractFile:  "LICENSE",
	})
	if err != nil {
		t.Fatalf("SelectExtractor(download with file): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "tool.tar.gz|tool|glob:LICENSE" {
		t.Fatalf("SelectExtractor(download with file) = %q", got)
	}
}

func TestSelectExtractorTreatsDownloadWithAllAsArchiveExtraction(t *testing.T) {
	svc := NewService()
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return &fakeChooser{name: "glob:" + pattern}, nil
	}
	svc.BinaryChooserFactory = func(tool string) any {
		return &fakeChooser{name: "binary:" + tool}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		ch := chooser.(*fakeChooser)
		return &fakeExtractor{name: filename + "|" + tool + "|" + ch.name}
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.tar.gz", "tool", &Options{
		DownloadOnly: true,
		All:          true,
	})
	if err != nil {
		t.Fatalf("SelectExtractor(download with all): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "tool.tar.gz|tool|glob:*" {
		t.Fatalf("SelectExtractor(download with all) = %q", got)
	}
}

func TestSelectExtractorRequiresSystem7zForExeExtractAll(t *testing.T) {
	svc := NewDefaultService(nil, nil)
	svc.System7zPathResolver = func(configured string) string {
		return ""
	}

	_, err := SelectExtractorAs[Extractor](svc, "https://example.com/qbittorrent_5.2.0_x64_setup.exe", "qbittorrent", &Options{
		All: true,
	})
	if err == nil {
		t.Fatal("expected exe extract-all without system 7z to fail")
	}
	if !strings.Contains(err.Error(), "system 7z is required") {
		t.Fatalf("expected missing system 7z error, got %v", err)
	}
}

func TestSelectExtractorRequiresSystem7zForExeFilePattern(t *testing.T) {
	svc := NewDefaultService(nil, nil)
	svc.System7zPathResolver = func(configured string) string {
		return ""
	}

	_, err := SelectExtractorAs[Extractor](svc, "https://example.com/qbittorrent_5.2.0_x64_setup.exe", "qbittorrent", &Options{
		ExtractFile: "qbittorrent.exe,*.dll",
	})
	if err == nil {
		t.Fatal("expected exe file pattern without system 7z to fail")
	}
	if !strings.Contains(err.Error(), "system 7z is required") {
		t.Fatalf("expected missing system 7z error, got %v", err)
	}
}

func TestSelectExtractorUsesSystem7zForExeExtractAll(t *testing.T) {
	svc := NewService()
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return NewFileChooser(pattern)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return &fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		return "C:/Tools/7z.exe"
	}
	svc.System7zExtractorFactory = func(filename, tool string, chooser Chooser, exe string) Extractor {
		return &fakeExtractor{name: "system7z:" + filepath.Base(filename)}
	}

	extractor, err := svc.SelectExtractor("https://example.com/setup.exe", "setup", &Options{All: true})
	if err != nil {
		t.Fatalf("SelectExtractor(exe extract-all): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "system7z:setup.exe" {
		t.Fatalf("expected system 7z extractor, got %q", got)
	}
}

func TestSelectExtractorUsesSystem7zForExeFilePattern(t *testing.T) {
	svc := NewService()
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return NewFileChooser(pattern)
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return &fakeExtractor{name: "go:" + filename}
	}
	svc.System7zPathResolver = func(configured string) string {
		return "C:/Tools/7z.exe"
	}
	svc.System7zExtractorFactory = func(filename, tool string, chooser Chooser, exe string) Extractor {
		return &fakeExtractor{name: "system7z:" + filepath.Base(filename)}
	}

	extractor, err := svc.SelectExtractor("https://example.com/setup.exe", "setup", &Options{ExtractFile: "bin/*.exe"})
	if err != nil {
		t.Fatalf("SelectExtractor(exe file pattern): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "system7z:setup.exe" {
		t.Fatalf("expected system 7z extractor, got %q", got)
	}
}

func TestSelectExtractorPrefersExplicitFilePatternsOverAll(t *testing.T) {
	svc := NewService()
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return &fakeChooser{name: "glob:" + pattern}, nil
	}
	svc.BinaryChooserFactory = func(tool string) any {
		return &fakeChooser{name: "binary:" + tool}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		ch := chooser.(*fakeChooser)
		return &fakeExtractor{name: filename + "|" + tool + "|" + ch.name}
	}

	extractor, err := svc.SelectExtractor("https://example.com/tool.tar.gz", "tool", &Options{
		DownloadOnly: true,
		ExtractFile:  "README,LICENSE",
		All:          true,
	})
	if err != nil {
		t.Fatalf("SelectExtractor(download with explicit file patterns): %v", err)
	}
	if got := extractor.(*fakeExtractor).name; got != "tool.tar.gz|tool|glob:README,LICENSE" {
		t.Fatalf("SelectExtractor(download with explicit file patterns) = %q", got)
	}
}
