package client

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/inherelab/eget/internal/util"
)

func resolvedAPICachePath(opts Options, rawURL string, parsed *url.URL) (string, bool) {
	if !opts.APICacheEnabled || !isProviderMetadataRequest(parsed) {
		return "", false
	}
	cacheDir := opts.APICacheDir
	if cacheDir == "" {
		return "", false
	}
	expanded, err := util.Expand(cacheDir)
	if err != nil {
		verbosef("api cache expand error: %v", err)
		return "", false
	}
	return APICacheFilePath(expanded, rawURL), true
}

func loadAPICacheResponse(path string, cacheTime int) (*http.Response, bool, error) {
	if path == "" {
		return nil, false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if cacheTime > 0 && time.Since(info.ModTime()) > time.Duration(cacheTime)*time.Second {
		verbosef("api cache expired: %s", path)
		return nil, false, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Body:          io.NopCloser(strings.NewReader(string(body))),
		ContentLength: int64(len(body)),
		Header:        make(http.Header),
	}, true, nil
}

func storeAPICacheResponse(path string, resp *http.Response) (*http.Response, error) {
	if path == "" || resp == nil || resp.Body == nil {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(strings.NewReader(string(body)))
	resp.ContentLength = int64(len(body))
	return resp, nil
}
