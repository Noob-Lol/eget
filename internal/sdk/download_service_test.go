package sdk

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestServiceDownloadExactVersionUsesURLTemplate(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.1", "go1.21.1.linux-amd64.zip")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://example.com/go1.21.1.linux-amd64.zip", req.URL)
			assert.Eq(t, "go1.21.1.linux-amd64.zip", req.Filename)
			assert.NotNil(t, req.Progress)
			return DownloadResult{Path: archivePath, Size: 7, FromCache: true}, nil
		},
	}

	result, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{
		Progress: func(size int64) io.Writer { return io.Discard },
	})

	assert.NoErr(t, err)
	assert.Eq(t, "go", result.Name)
	assert.Eq(t, "1.21.1", result.Version)
	assert.Eq(t, archivePath, result.Path)
	assert.Eq(t, "https://example.com/go1.21.1.linux-amd64.zip", result.URL)
	assert.Eq(t, "go1.21.1.linux-amd64.zip", result.Filename)
	assert.Eq(t, "linux", result.OS)
	assert.Eq(t, "amd64", result.Arch)
	assert.Eq(t, "zip", result.Ext)
	assert.True(t, result.Cached)
}

func TestServiceDownloadUsesIndexForPrefixVersion(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.13", "go1.21.13.linux-amd64.zip")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://mirror/go1.21.13.linux-amd64.zip", req.URL)
			return DownloadResult{Path: archivePath, Resumed: true}, nil
		},
	}
	assert.NoErr(t, svc.IndexCache.Save(Index{
		Schema:    1,
		SDK:       "go",
		SourceURL: "https://example.com/golang/",
		Items: []IndexItem{
			{Version: "1.21.1", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror/go1.21.1.linux-amd64.zip", Filename: "go1.21.1.linux-amd64.zip"}}},
			{Version: "1.21.13", Stable: true, Files: []IndexFile{{OS: "linux", Arch: "amd64", Ext: "zip", URL: "https://mirror/go1.21.13.linux-amd64.zip", Filename: "go1.21.13.linux-amd64.zip"}}},
		},
	}))

	result, err := svc.Download(context.Background(), "go:1.21", SDKDownloadOptions{})

	assert.NoErr(t, err)
	assert.Eq(t, "1.21.13", result.Version)
	assert.True(t, result.Resumed)
}

func TestServiceDownloadPlatformOverrideSelectsNonCurrentPlatform(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.1", "go1.21.1.windows-amd64.zip")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://example.com/go1.21.1.windows-amd64.zip", req.URL)
			return DownloadResult{Path: archivePath}, nil
		},
	}

	result, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{
		Platform: PlatformOptions{OS: "windows", Arch: "amd64"},
	})

	assert.NoErr(t, err)
	assert.Eq(t, "windows", result.OS)
	assert.Eq(t, "amd64", result.Arch)
	assert.Eq(t, "zip", result.Ext)
	assert.Eq(t, "go1.21.1.windows-amd64.zip", result.Filename)
}

func TestServiceDownloadPartialPlatformOverrideUsesCurrentDefault(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.1", "go1.21.1.windows-amd64.zip")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://example.com/go1.21.1.windows-amd64.zip", req.URL)
			return DownloadResult{Path: archivePath}, nil
		},
	}

	result, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{
		Platform: PlatformOptions{OS: "windows"},
	})

	assert.NoErr(t, err)
	assert.Eq(t, "windows", result.OS)
	assert.Eq(t, "amd64", result.Arch)
}

func TestServiceDownloadPartialArchOverrideUsesCurrentDefaultOS(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.1", "go1.21.1.linux-arm64.zip")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			assert.Eq(t, "https://example.com/go1.21.1.linux-arm64.zip", req.URL)
			return DownloadResult{Path: archivePath}, nil
		},
	}

	result, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{
		Platform: PlatformOptions{Arch: "arm64"},
	})

	assert.NoErr(t, err)
	assert.Eq(t, "linux", result.OS)
	assert.Eq(t, "arm64", result.Arch)
}

func TestServiceDownloadDoesNotRecordInstalledSDK(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "archive.zip")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			return DownloadResult{Path: archivePath}, nil
		},
	}

	_, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{})

	assert.NoErr(t, err)
	_, statErr := os.Stat(svc.Store.Path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestServiceDownloadCopiesToOutputDir(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "cache", "sdk-downloads", "go", "1.21.1", "go1.21.1.linux-amd64.zip")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	outputDir := filepath.Join(root, "downloads")
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			return DownloadResult{Path: archivePath, Size: 7}, nil
		},
	}

	result, err := svc.Download(context.Background(), "go@1.21.1", SDKDownloadOptions{OutputDir: outputDir})

	assert.NoErr(t, err)
	assert.Eq(t, filepath.Join(outputDir, "go1.21.1.linux-amd64.zip"), result.Path)
	data, err := os.ReadFile(result.Path)
	assert.NoErr(t, err)
	assert.Eq(t, "archive", string(data))
}

func TestServiceDownloadManyDownloadsTargetsInOrder(t *testing.T) {
	root := t.TempDir()
	var urls []string
	svc := Service{
		Config:     testSDKConfig(root),
		Store:      Store{Path: filepath.Join(root, "sdk.installed.json")},
		IndexCache: IndexCache{Dir: filepath.Join(root, "cache", "sdk-index")},
		GOOS:       "linux",
		GOARCH:     "amd64",
		Now:        time.Now,
		Downloader: func(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
			urls = append(urls, req.URL)
			return DownloadResult{Path: filepath.Join(root, req.Filename)}, nil
		},
	}

	results, err := svc.DownloadMany(context.Background(), []string{"go@1.21.1", "go@1.22.0"}, SDKDownloadOptions{})

	assert.NoErr(t, err)
	assert.Eq(t, 2, len(results))
	assert.Eq(t, []string{
		"https://example.com/go1.21.1.linux-amd64.zip",
		"https://example.com/go1.22.0.linux-amd64.zip",
	}, urls)
}
