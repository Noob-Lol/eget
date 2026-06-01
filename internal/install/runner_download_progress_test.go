package install

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

type recordingProgress struct {
	bytes    int
	finished bool
}

func TestDownloadWritesAndFinishesProgressWriter(t *testing.T) {
	origGetWithOptions := downloadGetWithOptions
	defer func() { downloadGetWithOptions = origGetWithOptions }()
	downloadGetWithOptions = func(url string, opts Options) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 12,
			Body:          io.NopCloser(strings.NewReader("network-data")),
		}, nil
	}

	progress := &recordingProgress{}
	var out bytes.Buffer
	err := Download("https://example.com/tool.tar.gz", &out, func(size int64) io.Writer {
		if size != 12 {
			t.Fatalf("expected content length 12, got %d", size)
		}
		return progress
	}, Options{})
	if err != nil {
		t.Fatalf("Download(): %v", err)
	}
	if out.String() != "network-data" {
		t.Fatalf("expected downloaded body, got %q", out.String())
	}
	if progress.bytes != len("network-data") {
		t.Fatalf("expected progress bytes %d, got %d", len("network-data"), progress.bytes)
	}
	if !progress.finished {
		t.Fatal("expected progress writer to be finished")
	}
}

func TestNewDownloadProgressUsesCoarseRedrawFrequency(t *testing.T) {
	p := newDownloadProgress(io.Discard, 500*1024*1024)
	defer p.Finish()

	assert.True(t, p.RedrawFreq >= 256*1024)
}

func TestDownloadProgressLayoutAdaptsToTerminalWidth(t *testing.T) {
	barWidth, format := downloadProgressLayout(120)
	assert.Eq(t, 40, barWidth)
	assert.Contains(t, format, "{@elapsed}/{@remaining}")
	assert.NotContains(t, format, "{@estimated}")

	barWidth, format = downloadProgressLayout(100)
	assert.Eq(t, 32, barWidth)
	assert.Contains(t, format, "{@elapsed}/{@remaining}")
	assert.NotContains(t, format, "{@estimated}")

	barWidth, format = downloadProgressLayout(80)
	assert.Eq(t, 24, barWidth)
	assert.Contains(t, format, "{@curSize}/{@maxSize}")
	assert.NotContains(t, format, "{@elapsed}")
	assert.NotContains(t, format, "{@remaining}")

	barWidth, _ = downloadProgressLayout(60)
	assert.Eq(t, 10, barWidth)
}
