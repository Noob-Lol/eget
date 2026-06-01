package cachemirror

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

const PathMD5Prefix = "path-md5:"

func KeyForRelPath(rel string) string {
	rel = normalizeRelPath(rel)
	sum := md5.Sum([]byte(rel))
	return PathMD5Prefix + hex.EncodeToString(sum[:])
}

func RelPath(cacheDir, fullPath string) (string, error) {
	root, err := filepath.Abs(cacheDir)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cache file %q is outside cache dir %q", fullPath, cacheDir)
	}
	return normalizeRelPath(rel), nil
}

func DownloadURL(baseURL, key string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("cache mirror url is empty")
	}
	if !strings.HasPrefix(key, PathMD5Prefix) {
		return "", fmt.Errorf("unsupported cache mirror key %q", key)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid cache mirror url %q", baseURL)
	}
	return baseURL + "/download/" + key, nil
}

func normalizeRelPath(rel string) string {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	return rel
}
