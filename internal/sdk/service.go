package sdk

import (
	"context"
	"io"
	"time"

	"github.com/inherelab/eget/internal/cachemirror"
	"github.com/inherelab/eget/internal/client"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

type DownloaderFunc func(context.Context, DownloadRequest) (DownloadResult, error)

type Service struct {
	Config      *cfgpkg.File
	Store       Store
	IndexCache  IndexCache
	ClientOpts  client.Options
	CacheMirror cachemirror.Options
	GOOS        string
	GOARCH      string
	Now         func() time.Time
	Downloader  DownloaderFunc

	OnIndexRefresh func(IndexRefreshEvent)
}

type InstallOptions struct {
	Force    bool
	Progress func(size int64) io.Writer
	OnStart  func(target string, version string, host string)
}

type PlatformOptions struct {
	OS   string
	Arch string
}

type SDKDownloadOptions struct {
	Platform  PlatformOptions
	OutputDir string
	Progress  func(size int64) io.Writer
	OnStart   func(target string, version string, host string, goos string, goarch string)
}

type SDKDownloadResult struct {
	Name     string
	Version  string
	Path     string
	URL      string
	Filename string
	OS       string
	Arch     string
	Ext      string
	Cached   bool
	Resumed  bool
}

type InstallResult struct {
	Name    string
	Version string
	Path    string
	URL     string
	Cached  bool
	Resumed bool
}

type RemoveResult struct {
	Name    string
	Version string
	Path    string
	Missing bool
}
