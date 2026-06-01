package install

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gookit/cliui/progress"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/gookit/goutil/x/termenv"
)

const downloadProgressRedrawFreq = 256 * 1024

func (r *InstallRunner) downloadBody(url string, opts Options) (downloadBodyResult, error) {
	cachePath := CacheFilePathWithMeta(opts.CacheDir, url, CacheMeta{Name: opts.CacheName, Version: opts.CacheVersion})
	output := r.Stdout
	if output == nil || opts.Quiet {
		output = io.Discard
	}
	if cachePath != "" && !IsLocalFile(url) {
		if data, err := os.ReadFile(cachePath); err == nil {
			if !isInvalidCachedDownload(cachePath, data) {
				ccolor.Fprintf(output, " - Using cached file <cyan>%s</>\n", filepath.Base(cachePath))
				return downloadBodyResult{Body: data, ModTime: fileModTime(cachePath)}, nil
			}
			verbosef("discard invalid cached archive: %s", cachePath)
		}
		result, err := DownloadFile(url, cachePath, r.downloadProgress(opts), opts)
		if err != nil {
			return downloadBodyResult{}, err
		}
		modTime := parseHTTPTime(result.LastModified)
		if !modTime.IsZero() {
			_ = applyModTime(cachePath, modTime)
		}
		data, err := os.ReadFile(cachePath)
		if err != nil {
			return downloadBodyResult{}, err
		}
		if modTime.IsZero() {
			modTime = fileModTime(cachePath)
		}
		return downloadBodyResult{Body: data, ModTime: modTime}, nil
	}

	buf := &bytes.Buffer{}
	result, err := DownloadWithResult(url, buf, r.downloadProgress(opts), opts)
	if err != nil {
		return downloadBodyResult{}, err
	}

	body := buf.Bytes()
	if cachePath != "" && !IsLocalFile(url) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
			_ = os.WriteFile(cachePath, body, 0o644)
		}
	}
	return downloadBodyResult{Body: body, ModTime: parseHTTPTime(result.LastModified)}, nil
}

func isInvalidCachedDownload(cachePath string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(cachePath))
	switch ext {
	case ".zip", ".gz", ".tgz", ".xz", ".bz2", ".zst", ".7z", ".rar":
	default:
		return false
	}
	trimmed := bytes.TrimSpace(data)
	lowerPrefix := strings.ToLower(string(trimmed[:min(len(trimmed), 64)]))
	return strings.HasPrefix(lowerPrefix, "<!doctype html") || strings.HasPrefix(lowerPrefix, "<html")
}

func parseHTTPTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := http.ParseTime(value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func (r *InstallRunner) downloadProgress(opts Options) func(int64) io.Writer {
	return func(size int64) io.Writer {
		pbout := r.Stdout
		if pbout == nil || opts.Quiet {
			pbout = io.Discard
		}
		return newDownloadProgress(pbout, size)
	}
}

func newDownloadProgress(out io.Writer, size int64) *progress.Progress {
	return NewDownloadProgress(out, size)
}

func NewDownloadProgress(out io.Writer, size int64) *progress.Progress {
	if out == nil {
		out = io.Discard
	}
	width, _ := termenv.GetTermSize()
	barWidth, format := downloadProgressLayout(width)
	p := progress.CustomBar(barWidth, progress.BarStyles[0], size)
	p.Out = out
	p.RedrawFreq = downloadProgressRedrawFreq
	p.Format = format
	p.Start()
	return p
}

func downloadProgressLayout(termWidth int) (int, string) {
	if termWidth <= 0 {
		termWidth = 80
	}
	format := "Downloading [{@bar}] <info>{@percent:4s}%</> {@curSize}/{@maxSize}"
	if termWidth >= 120 {
		return 40, format + " ({@elapsed}/{@remaining})"
	}
	if termWidth >= 100 {
		return 32, format + " ({@elapsed}/{@remaining})"
	}
	if termWidth >= 80 {
		return 24, format
	}
	if termWidth >= 64 {
		return 16, format
	}
	return 10, format
}
