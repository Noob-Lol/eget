package install

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	sourcesf "github.com/inherelab/eget/internal/source/sourceforge"
)

func TestResolveCandidateSelectsUniqueNameMatch(t *testing.T) {
	runner := &InstallRunner{Stderr: io.Discard}
	runner.Prompt = func(title, filterPrompt string, choices []string) (int, error) {
		t.Fatalf("expected --name to avoid prompt, got choices %#v", choices)
		return 0, nil
	}

	got, err := runner.resolveCandidate("gookit/greq", []string{
		"https://github.com/gookit/greq/releases/download/v0.6.0/gbench-v0.6.0-windows-amd64.zip",
		"https://github.com/gookit/greq/releases/download/v0.6.0/greq-v0.6.0-windows-amd64.zip",
	}, Options{Name: "gbench"}, "")

	assert.NoErr(t, err)
	assert.Eq(t, "https://github.com/gookit/greq/releases/download/v0.6.0/gbench-v0.6.0-windows-amd64.zip", got)
}

func TestResolveCandidateKeepsPromptWhenNameMatchIsAmbiguous(t *testing.T) {
	runner := &InstallRunner{Stderr: io.Discard}
	prompted := false
	runner.Prompt = func(title, filterPrompt string, choices []string) (int, error) {
		prompted = true
		assert.Eq(t, "Select package resource v0.6.0", title)
		assert.Eq(t, "Filter assets", filterPrompt)
		assert.Eq(t, []string{
			"gbench-v0.6.0-windows-amd64.zip",
			"gbench-lite-v0.6.0-windows-amd64.zip",
		}, choices)
		return 1, nil
	}

	got, err := runner.resolveCandidate("gookit/greq", []string{
		"https://github.com/gookit/greq/releases/download/v0.6.0/gbench-v0.6.0-windows-amd64.zip",
		"https://github.com/gookit/greq/releases/download/v0.6.0/gbench-lite-v0.6.0-windows-amd64.zip",
	}, Options{Name: "gbench"}, "v0.6.0")

	assert.NoErr(t, err)
	assert.True(t, prompted)
	assert.Eq(t, "https://github.com/gookit/greq/releases/download/v0.6.0/gbench-lite-v0.6.0-windows-amd64.zip", got)
}

func TestResolveExtractedFileUsesExtractedFilePromptTitle(t *testing.T) {
	runner := &InstallRunner{Stderr: io.Discard}
	prompted := false
	runner.Prompt = func(title, filterPrompt string, choices []string) (int, error) {
		prompted = true
		assert.Eq(t, "Select extracted file", title)
		assert.Eq(t, "Filter files", filterPrompt)
		assert.Eq(t, []string{"gsa.exe", "gsa-helper.exe", "all"}, choices)
		return 0, nil
	}

	selected, all, err := runner.resolveExtractedFile([]ExtractedFile{
		{ArchiveName: `gsa.exe`, Name: `gsa.exe`, mode: 0o666},
		{ArchiveName: `gsa-helper.exe`, Name: `gsa-helper.exe`, mode: 0o666},
	}, Options{System: "windows/amd64"})

	assert.NoErr(t, err)
	assert.True(t, prompted)
	assert.False(t, all)
	assert.Eq(t, `gsa.exe`, selected.ArchiveName)
}

func TestRunFallsBackToOlderSourceForgeVersionWhenAssetMissing(t *testing.T) {
	responses := map[string]string{
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/": `
<script>
net.sf.files = {
  "2.59": {"name":"2.59","full_path":"/Translations 2.x/2.59","type":"d"},
  "2.60": {"name":"2.60","full_path":"/Translations 2.x/2.60","type":"d"},
  "2.61": {"name":"2.61","full_path":"/Translations 2.x/2.61","type":"d"}
};
</script>`,
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.61/": `
<script>
net.sf.files = {
  "Spanish.zip": {
    "name":"Spanish.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.61/Spanish.zip",
    "full_path":"/Translations 2.x/2.61/Spanish.zip",
    "type":"f"
  }
};
</script>`,
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.60/": `
<script>
net.sf.files = {
  "German.zip": {
    "name":"German.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.60/German.zip",
    "full_path":"/Translations 2.x/2.60/German.zip",
    "type":"f"
  }
};
</script>`,
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.59/": `
<script>
net.sf.files = {
  "Ukrainian.zip": {
    "name":"Ukrainian.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.59/Ukrainian.zip",
    "full_path":"/Translations 2.x/2.59/Ukrainian.zip",
    "type":"f"
  }
};
</script>`,
	}
	var sourceForgeRequests []string
	svc := NewDefaultService(nil, nil)
	svc.SourceForgeGetterFactory = func(opts Options) sourcesf.HTTPGetter {
		return HTTPGetterFunc(func(url string) (*http.Response, error) {
			sourceForgeRequests = append(sourceForgeRequests, url)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responses[url])),
			}, nil
		})
	}

	var downloadedURL string
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		downloadedURL = url
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("translation")),
		}, nil
	}

	outputDir := t.TempDir()
	runner := NewRunner(svc)
	runner.Stderr = io.Discard
	result, err := runner.Run("sourceforge:keepass/Translations 2.x", Options{
		FallbackVersions: 10,
		Asset:            []string{"Ukrainian", "zip"},
		DownloadOnly:     true,
		Output:           outputDir,
	})

	if err != nil {
		t.Fatalf("run sourceforge fallback: %v", err)
	}
	wantURL := "https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.59/Ukrainian.zip"
	if result.URL != wantURL || downloadedURL != wantURL {
		t.Fatalf("expected fallback URL %q, got result=%q downloaded=%q", wantURL, result.URL, downloadedURL)
	}
	assertRequests := []string{
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/",
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.61/",
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/",
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.60/",
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.59/",
	}
	if strings.Join(sourceForgeRequests, "\n") != strings.Join(assertRequests, "\n") {
		t.Fatalf("unexpected sourceforge requests:\n%v", strings.Join(sourceForgeRequests, "\n"))
	}
}
