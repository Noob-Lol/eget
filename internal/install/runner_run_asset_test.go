package install

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestRunAssetRunsVerifiedDownloadedAsset(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	origDownloadGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origDownloadGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		assert.Eq(t, assetURL, url)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer-data")),
		}, nil
	}

	svc := newRunAssetTestService(t, assetURL)
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard

	var launchedPath string
	var launchedArgs []string
	runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
		launchedPath = path
		launchedArgs = append([]string(nil), args...)
		data, err := os.ReadFile(path)
		assert.NoErr(t, err)
		assert.Eq(t, "installer-data", string(data))
		assert.NotNil(t, stdout)
		assert.NotNil(t, stderr)
		return nil
	}

	result, err := runner.Run("template:claude", Options{
		System:   "windows/amd64",
		CacheDir: t.TempDir(),
		Verify:   "abc",
		URLTemplate: URLTemplateOptions{
			URLTemplate:   "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			InstallAction: InstallActionRunAsset,
			InstallArgs:   []string{"install", "latest"},
		},
		Tag: "1.2.3",
	})
	if err != nil {
		t.Fatalf("run asset: %v", err)
	}

	assert.Contains(t, filepath.Base(launchedPath), "claude")
	assert.Eq(t, []string{"install", "latest"}, launchedArgs)
	assert.Eq(t, InstallModeRunAsset, result.InstallMode)
	assert.Eq(t, "1.2.3", result.Version)
	assert.Eq(t, assetURL, result.URL)
}

func TestRunAssetDownloadOnlyDoesNotRunAsset(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	origDownloadGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origDownloadGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		assert.Eq(t, assetURL, url)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer-data")),
		}, nil
	}

	svc := newRunAssetTestService(t, assetURL)
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
		t.Fatal("asset runner should not be called for download-only")
		return nil
	}
	outputDir := t.TempDir()

	result, err := runner.Run("template:claude", Options{
		System:       "windows/amd64",
		Output:       outputDir,
		DownloadOnly: true,
		CacheDir:     t.TempDir(),
		Verify:       "abc",
		URLTemplate: URLTemplateOptions{
			URLTemplate:   "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			InstallAction: InstallActionRunAsset,
			InstallArgs:   []string{"install", "latest"},
		},
		Tag: "1.2.3",
	})
	if err != nil {
		t.Fatalf("download run asset: %v", err)
	}

	wantPath := filepath.Join(outputDir, "claude.exe")
	assert.Eq(t, []string{wantPath}, result.ExtractedFiles)
	data, err := os.ReadFile(wantPath)
	assert.NoErr(t, err)
	assert.Eq(t, "installer-data", string(data))
	assert.Eq(t, "", result.InstallMode)
}

func TestRunAssetDoesNotRunWhenChecksumFails(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	origDownloadGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origDownloadGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer-data")),
		}, nil
	}

	svc := newRunAssetTestService(t, assetURL)
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		return failingVerifier{}, nil
	}
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
		t.Fatal("asset runner should not be called after checksum failure")
		return nil
	}

	_, err := runner.Run("template:claude", Options{
		System:   "windows/amd64",
		CacheDir: t.TempDir(),
		Verify:   "abc",
		URLTemplate: URLTemplateOptions{
			URLTemplate:   "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			InstallAction: InstallActionRunAsset,
		},
		Tag: "1.2.3",
	})

	if err == nil || !strings.Contains(err.Error(), "checksum failed") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
}

func TestRunAssetRequiresChecksumSource(t *testing.T) {
	assetURL := "https://example.com/1.2.3/win32-x64/claude.exe"
	svc := newRunAssetTestService(t, assetURL)
	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	runner.AssetRunner = func(path string, args []string, stdout, stderr io.Writer) error {
		t.Fatal("asset runner should not be called without checksum source")
		return nil
	}

	_, err := runner.Run("template:claude", Options{
		System:   "windows/amd64",
		CacheDir: t.TempDir(),
		URLTemplate: URLTemplateOptions{
			URLTemplate:   "https://example.com/{version}/{os}-{arch}/claude{ext}",
			OSMap:         map[string]string{"windows": "win32"},
			ArchMap:       map[string]string{"amd64": "x64"},
			ExtMap:        map[string]string{"windows": ".exe"},
			InstallAction: InstallActionRunAsset,
		},
		Tag: "1.2.3",
	})

	if err == nil || !strings.Contains(err.Error(), "requires checksum") {
		t.Fatalf("expected checksum source error, got %v", err)
	}
}

func newRunAssetTestService(t *testing.T, assetURL string) *Service {
	t.Helper()
	svc := NewService()
	svc.SystemDetectorFactory = func(goos, goarch string) (Detector, error) {
		assert.Eq(t, "windows", goos)
		assert.Eq(t, "amd64", goarch)
		return &fakeDetector{name: assetURL}, nil
	}
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		assert.Eq(t, "abc", expected)
		return &fakeVerifier{}, nil
	}
	svc.DownloadOnlyExtractorFactory = func(name string) any {
		return NewDownloadOnlyExtractor(name)
	}
	return svc
}
