package sdk

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/inherelab/eget/internal/client"
)

type DownloadRequest struct {
	URL      string
	CacheDir string
	SDK      string
	Version  string
	Filename string

	ClientOpts client.Options
	Progress   func(size int64) io.Writer
}

type DownloadResult struct {
	Path      string
	FromCache bool
	Resumed   bool
	Size      int64
	ETag      string
	Modified  string
}

type downloadMeta struct {
	Schema       int       `json:"schema"`
	URL          string    `json:"url"`
	Filename     string    `json:"filename"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type remoteDownloadInfo struct {
	Size         int64
	AcceptRange  bool
	ETag         string
	LastModified string
}

func DownloadArchive(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
	finalPath := sdkDownloadFinalPath(req)
	partPath := sdkDownloadPartPath(req)
	metaPath := sdkDownloadMetaPath(req)

	if ok, meta := completeCacheMatches(finalPath, metaPath, req); ok {
		return DownloadResult{
			Path:      finalPath,
			FromCache: true,
			Size:      meta.Size,
			ETag:      meta.ETag,
			Modified:  meta.LastModified,
		}, nil
	}

	httpClient, err := newDownloadHTTPClient(req.ClientOpts)
	if err != nil {
		return DownloadResult{}, err
	}
	remote, err := probeDownload(ctx, httpClient, req.URL)
	if err != nil {
		return DownloadResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		return DownloadResult{}, err
	}

	resumed := false
	partSize := fileSize(partPath)
	meta, metaOK := loadDownloadMeta(metaPath)
	canResume := partSize > 0 &&
		remote.AcceptRange &&
		(remote.Size <= 0 || partSize < remote.Size) &&
		metaOK &&
		metaMatchesRemote(meta, req, remote)

	if !canResume {
		_ = os.Remove(partPath)
		partSize = 0
	}

	resp, err := downloadRequest(ctx, httpClient, req.URL, partSize)
	if err != nil {
		return DownloadResult{}, err
	}
	defer resp.Body.Close()

	switch {
	case partSize > 0 && resp.StatusCode == http.StatusPartialContent:
		resumed = true
		err = appendResponseToFile(partPath, resp.Body, req.Progress, remote.Size)
	case partSize > 0 && resp.StatusCode == http.StatusOK:
		err = writeResponseToFile(partPath, resp.Body, req.Progress, remote.Size)
	default:
		if resp.StatusCode != http.StatusOK {
			return DownloadResult{}, fmt.Errorf("download error: %d", resp.StatusCode)
		}
		err = writeResponseToFile(partPath, resp.Body, req.Progress, remote.Size)
	}
	if err != nil {
		_ = os.Remove(partPath)
		return DownloadResult{}, err
	}

	size := fileSize(partPath)
	if remote.Size > 0 && size != remote.Size {
		_ = os.Remove(partPath)
		return DownloadResult{}, fmt.Errorf("download size mismatch: expected %d, got %d", remote.Size, size)
	}
	meta = downloadMeta{
		Schema:       1,
		URL:          req.URL,
		Filename:     req.Filename,
		Size:         size,
		ETag:         remote.ETag,
		LastModified: remote.LastModified,
		UpdatedAt:    time.Now(),
	}
	if err := saveDownloadMeta(metaPath, meta); err != nil {
		return DownloadResult{}, err
	}
	if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
		return DownloadResult{}, err
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{
		Path:     finalPath,
		Resumed:  resumed,
		Size:     size,
		ETag:     meta.ETag,
		Modified: meta.LastModified,
	}, nil
}

func sdkDownloadFinalPath(req DownloadRequest) string {
	return filepath.Join(req.CacheDir, "sdk-downloads", safeName(req.SDK), safeName(req.Version), req.Filename)
}

func sdkDownloadPartPath(req DownloadRequest) string {
	return sdkDownloadFinalPath(req) + ".part"
}

func sdkDownloadMetaPath(req DownloadRequest) string {
	return sdkDownloadFinalPath(req) + ".meta.json"
}

func completeCacheMatches(path, metaPath string, req DownloadRequest) (bool, downloadMeta) {
	meta, ok := loadDownloadMeta(metaPath)
	if !ok || meta.URL != req.URL || meta.Filename != req.Filename {
		return false, downloadMeta{}
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() != meta.Size {
		return false, downloadMeta{}
	}
	return true, meta
}

func loadDownloadMeta(path string) (downloadMeta, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return downloadMeta{}, false
	}
	var meta downloadMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return downloadMeta{}, false
	}
	return meta, true
}

func saveDownloadMeta(path string, meta downloadMeta) error {
	if meta.Schema == 0 {
		meta.Schema = 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func metaMatchesRemote(meta downloadMeta, req DownloadRequest, remote remoteDownloadInfo) bool {
	if meta.URL != req.URL || meta.Filename != req.Filename {
		return false
	}
	if remote.Size > 0 && meta.Size != remote.Size {
		return false
	}
	if remote.ETag != "" && meta.ETag != "" && remote.ETag != meta.ETag {
		return false
	}
	if remote.LastModified != "" && meta.LastModified != "" && remote.LastModified != meta.LastModified {
		return false
	}
	return true
}

func newDownloadHTTPClient(opts client.Options) (*http.Client, error) {
	proxyFunc, err := client.ProxyFuncFor(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: &http.Transport{
		Proxy:           proxyFunc,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.DisableSSL},
	}}, nil
}

func probeDownload(ctx context.Context, client *http.Client, rawURL string) (remoteDownloadInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return remoteDownloadInfo{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return remoteDownloadInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return remoteDownloadInfo{}, fmt.Errorf("download probe error: %d", resp.StatusCode)
	}
	size := resp.ContentLength
	if size < 0 {
		size, _ = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	}
	return remoteDownloadInfo{
		Size:         size,
		AcceptRange:  strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

func downloadRequest(ctx context.Context, client *http.Client, rawURL string, offset int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	return client.Do(req)
}

func appendResponseToFile(path string, body io.Reader, progress func(int64) io.Writer, size int64) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(io.MultiWriter(out, progressWriter(progress, size)), body)
	return err
}

func writeResponseToFile(path string, body io.Reader, progress func(int64) io.Writer, size int64) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(io.MultiWriter(out, progressWriter(progress, size)), body)
	return err
}

func progressWriter(progress func(int64) io.Writer, size int64) io.Writer {
	if progress == nil {
		return io.Discard
	}
	writer := progress(size)
	if writer == nil {
		return io.Discard
	}
	return writer
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}
