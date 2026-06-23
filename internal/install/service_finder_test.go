package install

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	forge "github.com/inherelab/eget/internal/source/forge"
	sourcegithub "github.com/inherelab/eget/internal/source/github"
	sourcesf "github.com/inherelab/eget/internal/source/sourceforge"
	"github.com/inherelab/eget/internal/source/urltemplate"
)

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

	t.Run("pkg-template target", func(t *testing.T) {
		svc.TemplateGetterFactory = nil
		finder, tool, err := svc.SelectFinder("pkg-template:mydev:markview", &Options{
			URLTemplate: URLTemplateOptions{
				LatestURL:   "http://mydev.lan/tools/markview/latest.yaml",
				URLTemplate: "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}",
			},
		})
		if err != nil {
			t.Fatalf("SelectFinder(pkg-template): %v", err)
		}
		assert.Eq(t, "markview", tool)
		got, ok := finder.(*urltemplate.Finder)
		if !ok {
			t.Fatalf("finder type = %T, want *urltemplate.Finder", finder)
		}
		assert.Eq(t, "markview", got.Name)
		assert.Eq(t, "http://mydev.lan/tools/markview/latest.yaml", got.Config.LatestURL)
		assert.Eq(t, "http://mydev.lan/tools/{name}/{name}-{os}-{arch}{ext}", got.Config.URLTemplate)
	})

	t.Run("invalid target", func(t *testing.T) {
		if _, _, err := svc.SelectFinder("invalid-target", &Options{}); err == nil {
			t.Fatal("expected invalid target error")
		}
	})
}
