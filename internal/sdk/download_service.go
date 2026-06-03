package sdk

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (s Service) Download(ctx context.Context, rawTarget string, opts SDKDownloadOptions) (SDKDownloadResult, error) {
	archive, err := s.resolveDownloadArchive(rawTarget, opts.Platform)
	if err != nil {
		return SDKDownloadResult{}, err
	}
	if opts.OnStart != nil {
		host := indexSourceHost(archive.URL)
		if host == "" {
			host = archive.URL
		}
		opts.OnStart(rawTarget, archive.Version, host)
	}
	download := s.Downloader
	if download == nil {
		download = DownloadArchive
	}
	downloadResult, err := download(ctx, DownloadRequest{
		URL:         archive.URL,
		CacheDir:    s.cacheDir(),
		SDK:         archive.Config.Name,
		Version:     archive.Version,
		Filename:    archive.Filename,
		ClientOpts:  s.effectiveClientOptions(),
		CacheMirror: s.CacheMirror,
		Progress:    opts.Progress,
	})
	if err != nil {
		return SDKDownloadResult{}, err
	}
	path := downloadResult.Path
	if opts.OutputDir != "" {
		path, err = copyDownloadedArchive(downloadResult.Path, opts.OutputDir, archive.Filename)
		if err != nil {
			return SDKDownloadResult{}, err
		}
	}
	return SDKDownloadResult{
		Name:     archive.Config.Name,
		Version:  archive.Version,
		Path:     path,
		URL:      archive.URL,
		Filename: archive.Filename,
		OS:       archive.OS,
		Arch:     archive.Arch,
		Ext:      archive.Ext,
		Cached:   downloadResult.FromCache,
		Resumed:  downloadResult.Resumed,
	}, nil
}

func (s Service) DownloadMany(ctx context.Context, targets []string, opts SDKDownloadOptions) ([]SDKDownloadResult, error) {
	results := make([]SDKDownloadResult, 0, len(targets))
	for _, target := range targets {
		result, err := s.Download(ctx, target, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func copyDownloadedArchive(src, outputDir, filename string) (string, error) {
	if info, err := os.Stat(outputDir); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("sdk download output path is not a directory: %s", outputDir)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	dst := filepath.Join(outputDir, filename)
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return dst, nil
}
