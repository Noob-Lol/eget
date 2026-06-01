package sdk

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/inherelab/eget/internal/cachemirror"
	"github.com/inherelab/eget/internal/client"
)

type DownloadRequest struct {
	URL      string
	CacheDir string
	SDK      string
	Version  string
	Filename string

	ClientOpts  client.Options
	CacheMirror cachemirror.Options
	Progress    func(size int64) io.Writer
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

func DownloadArchive(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
	finalPath := sdkDownloadFinalPath(req)
	metaPath := sdkDownloadMetaPath(req)

	if ctx != nil {
		select {
		case <-ctx.Done():
			return DownloadResult{}, ctx.Err()
		default:
		}
	}

	if ok, meta := completeCacheMatches(finalPath, metaPath, req); ok {
		return DownloadResult{
			Path:      finalPath,
			FromCache: true,
			Size:      meta.Size,
			ETag:      meta.ETag,
			Modified:  meta.LastModified,
		}, nil
	}

	result, err := client.DownloadFile(req.URL, finalPath, req.Progress, req.ClientOpts)
	if err != nil {
		return DownloadResult{}, err
	}
	meta := downloadMeta{
		Schema:       1,
		URL:          req.URL,
		Filename:     req.Filename,
		Size:         result.Size,
		ETag:         result.ETag,
		LastModified: result.LastModified,
		UpdatedAt:    time.Now(),
	}
	if err := saveDownloadMeta(metaPath, meta); err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{
		Path:     finalPath,
		Resumed:  result.Resumed,
		Size:     result.Size,
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
