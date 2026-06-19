package install

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestRunExtractAllUsesDirectExtractorWithoutListing(t *testing.T) {
	assetURL := "https://example.com/tool.7z"
	outputDir := t.TempDir()
	direct := &fakeDirectAllExtractor{
		files: []string{filepath.Join(outputDir, "$PLUGINSDIR", "nsDialogs.dll")},
	}

	svc := NewService()
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return direct
	}
	svc.System7zPathResolver = func(configured string) string {
		return ""
	}
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return NewFileChooser(pattern)
	}
	svc.NoVerifierFactory = func() Verifier {
		return &fakeVerifier{}
	}

	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("installer")),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stderr = io.Discard
	result, err := runner.Run(assetURL, Options{All: true, Output: outputDir})
	if err != nil {
		t.Fatalf("run extract-all: %v", err)
	}

	if direct.extractCalled {
		t.Fatal("expected direct extract-all to skip Extract")
	}
	if len(result.ExtractedFiles) != 1 {
		t.Fatalf("expected extracted files, got %#v", result.ExtractedFiles)
	}
}

func TestRunExtractAllPassesStripComponentsToDirectExtractor(t *testing.T) {
	assetURL := "https://example.com/ventoy.zip"
	outputDir := t.TempDir()
	direct := &fakeDirectAllExtractor{
		files: []string{filepath.Join(outputDir, "Ventoy2Disk.exe")},
	}

	svc := NewService()
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	svc.ExtractorFactory = func(filename, tool string, chooser any) any {
		return direct
	}
	svc.System7zPathResolver = func(configured string) string {
		return ""
	}
	svc.GlobChooserFactory = func(pattern string) (any, error) {
		return NewFileChooser(pattern)
	}
	svc.NoVerifierFactory = func() Verifier {
		return &fakeVerifier{}
	}

	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("zip")),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stderr = io.Discard
	_, err := runner.Run(assetURL, Options{All: true, Output: outputDir, StripComponents: 1})
	if err != nil {
		t.Fatalf("run extract-all: %v", err)
	}

	assert.Eq(t, 1, direct.strip)
}

func TestRunExtractAllStripsComponentsFromZipArchive(t *testing.T) {
	assetURL := "https://example.com/ventoy.zip"
	archive := zipBytes(t, map[string]string{
		"ventoy-1.1.12/Ventoy2Disk.exe": "exe",
		"ventoy-1.1.12/boot/boot.img":   "boot",
	})
	outputDir := t.TempDir()

	svc := NewDefaultService(nil, nil)
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(archive)),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	result, err := runner.Run(assetURL, Options{All: true, Output: outputDir, StripComponents: 1})
	if err != nil {
		t.Fatalf("run extract-all: %v", err)
	}

	assert.Eq(t, 2, len(result.ExtractedFiles))
	data, err := os.ReadFile(filepath.Join(outputDir, "Ventoy2Disk.exe"))
	if err != nil {
		t.Fatalf("read stripped exe: %v", err)
	}
	assert.Eq(t, "exe", string(data))
	data, err = os.ReadFile(filepath.Join(outputDir, "boot", "boot.img"))
	if err != nil {
		t.Fatalf("read stripped boot file: %v", err)
	}
	assert.Eq(t, "boot", string(data))
	if _, err := os.Stat(filepath.Join(outputDir, "ventoy-1.1.12")); !os.IsNotExist(err) {
		t.Fatalf("expected stripped root directory to be absent, stat err=%v", err)
	}
}

func TestRunPortableGUIArchiveExtractsAllToNamedGuiTarget(t *testing.T) {
	assetURL := "https://example.com/v2rayN-windows-64-desktop.zip"
	archive := zipBytes(t, map[string]string{
		"v2rayN-windows-64/v2rayN.exe":             "exe",
		"v2rayN-windows-64/bin/xray/xray.exe":      "xray",
		"v2rayN-windows-64/guiConfigs/config.json": "config",
	})
	guiTarget := t.TempDir()

	svc := NewDefaultService(nil, nil)
	svc.AllDetectorFactory = func() Detector {
		return &fakeDetector{name: assetURL}
	}
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(archive)),
		}, nil
	}

	runner := NewRunner(svc)
	runner.Stdout = io.Discard
	runner.Stderr = io.Discard
	result, err := runner.Run(assetURL, Options{
		Name:        "v2rayn",
		GuiTarget:   guiTarget,
		IsGUI:       true,
		InstallMode: InstallModePortable,
	})
	if err != nil {
		t.Fatalf("run portable gui archive: %v", err)
	}

	wantRoot := filepath.Join(guiTarget, "v2rayn")
	assert.Eq(t, InstallModePortable, result.InstallMode)
	assert.True(t, result.IsGUI)
	assert.Eq(t, 3, len(result.ExtractedFiles))
	for _, name := range []string{
		filepath.Join(wantRoot, "v2rayN-windows-64", "v2rayN.exe"),
		filepath.Join(wantRoot, "v2rayN-windows-64", "bin", "xray", "xray.exe"),
		filepath.Join(wantRoot, "v2rayN-windows-64", "guiConfigs", "config.json"),
	} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected extracted file %s: %v", name, err)
		}
	}
}
