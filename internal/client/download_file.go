package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var errDownloadRangeReturnedOK = errors.New("download range returned ok")

type DownloadFileResult struct {
	Path         string
	FromCache    bool
	Resumed      bool
	Parallel     bool
	Size         int64
	ETag         string
	LastModified string
}

type downloadFileMeta struct {
	Schema       int                 `json:"schema"`
	URL          string              `json:"url"`
	Size         int64               `json:"size"`
	ETag         string              `json:"etag,omitempty"`
	LastModified string              `json:"last_modified,omitempty"`
	ChunkSize    int64               `json:"chunk_size,omitempty"`
	Chunks       []downloadChunkMeta `json:"chunks,omitempty"`
	UpdatedAt    time.Time           `json:"updated_at,omitempty"`
}

type downloadChunkMeta struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  bool  `json:"done"`
}

type downloadFileRemote struct {
	Size         int64
	AcceptRange  bool
	ETag         string
	LastModified string
}

func DownloadFile(rawURL, target string, getbar func(size int64) io.Writer, opts Options) (DownloadFileResult, error) {
	if isLocalFile(rawURL) {
		return downloadLocalFile(rawURL, target)
	}

	if shouldUseConfiguredProxy(downloadNoticeURL(rawURL, opts), opts.ProxyURL, opts.ProxyExclude) {
		printProxyNotice("download request", opts.ProxyURL)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return DownloadFileResult{}, err
	}
	partPath := target + ".part"
	metaPath := target + ".meta.json"
	remote, remoteOK := probeDownloadFile(rawURL, opts)
	if remoteOK && remote.AcceptRange && shouldUseParallelDownloadFile(remote.Size, metaPath, opts) {
		chunks := effectiveFileChunkCount(opts.ChunkConcurrency, remote.Size)
		if chunks > 1 {
			return downloadFileParallel(rawURL, target, partPath, metaPath, remote, getbar, opts, chunks)
		}
	}
	return downloadFileSingle(rawURL, target, partPath, metaPath, getbar, opts)
}

func shouldUseParallelDownloadFile(size int64, metaPath string, opts Options) bool {
	return size > resumableDownloadMinSize || hasParallelDownloadFileMeta(metaPath) || opts.ChunkConcurrency > 1
}

func hasParallelDownloadFileMeta(metaPath string) bool {
	meta, ok := loadDownloadFileMeta(metaPath)
	return ok && meta.Schema == 2 && len(meta.Chunks) > 0
}

func downloadLocalFile(rawURL, target string) (DownloadFileResult, error) {
	in, err := os.Open(rawURL)
	if err != nil {
		return DownloadFileResult{}, err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return DownloadFileResult{}, err
	}
	out, err := os.Create(target)
	if err != nil {
		return DownloadFileResult{}, err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return DownloadFileResult{}, copyErr
	}
	if closeErr != nil {
		return DownloadFileResult{}, closeErr
	}
	return DownloadFileResult{Path: target, Size: fileSize(target)}, nil
}

func downloadFileSingle(rawURL, target, partPath, metaPath string, getbar func(int64) io.Writer, opts Options) (DownloadFileResult, error) {
	partSize := fileSize(partPath)
	var remote downloadFileRemote
	if partSize > 0 {
		if probed, ok := probeDownloadFile(rawURL, opts); ok {
			remote = probed
		}
	}
	if partSize > 0 && canResumeDownloadFile(rawURL, partSize, metaPath, remote) {
		if partSize == remote.Size {
			if err := replaceFile(partPath, target); err != nil {
				return DownloadFileResult{}, err
			}
			return DownloadFileResult{Path: target, Resumed: true, Size: remote.Size, ETag: remote.ETag, LastModified: remote.LastModified}, nil
		}
		resp, err := requestWithOptions(http.MethodGet, rawURL, fmt.Sprintf("bytes=%d-", partSize), opts)
		if err != nil {
			return DownloadFileResult{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusPartialContent {
			bar := downloadProgressWriter(getbar, remote.Size)
			if err := writeDownloadResponse(partPath, resp.Body, bar, true); err != nil {
				return DownloadFileResult{}, err
			}
			if remote.Size > 0 && fileSize(partPath) != remote.Size {
				return DownloadFileResult{}, fmt.Errorf("download size mismatch: expected %d, got %d", remote.Size, fileSize(partPath))
			}
			if err := replaceFile(partPath, target); err != nil {
				return DownloadFileResult{}, err
			}
			_ = os.Remove(metaPath)
			if finisher, ok := bar.(progressFinisher); ok {
				finisher.Finish()
			}
			return DownloadFileResult{Path: target, Resumed: true, Size: remote.Size, ETag: remote.ETag, LastModified: remote.LastModified}, nil
		}
		if resp.StatusCode != http.StatusOK {
			return DownloadFileResult{}, fmt.Errorf("download error: %d", resp.StatusCode)
		}
	}

	resp, err := downloadGetWithOptions(rawURL, opts)
	if err != nil {
		return DownloadFileResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return DownloadFileResult{}, err
		}
		return DownloadFileResult{}, fmt.Errorf("download error: %d: %s", resp.StatusCode, body)
	}

	size := resp.ContentLength
	resumable := size > resumableDownloadMinSize && strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")
	bar := downloadProgressWriter(getbar, size)
	if err := writeDownloadResponse(partPath, resp.Body, bar, false); err != nil {
		if resumable {
			_ = saveDownloadFileMeta(metaPath, downloadFileMeta{
				Schema:       1,
				URL:          rawURL,
				Size:         size,
				ETag:         resp.Header.Get("ETag"),
				LastModified: resp.Header.Get("Last-Modified"),
			})
		} else {
			_ = os.Remove(partPath)
			_ = os.Remove(metaPath)
		}
		return DownloadFileResult{}, err
	}
	if err := replaceFile(partPath, target); err != nil {
		return DownloadFileResult{}, err
	}
	_ = os.Remove(metaPath)
	if finisher, ok := bar.(progressFinisher); ok {
		finisher.Finish()
	}
	return DownloadFileResult{Path: target, Size: fileSize(target), ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}, nil
}

func downloadFileParallel(rawURL, target, partPath, metaPath string, remote downloadFileRemote, getbar func(int64) io.Writer, opts Options, chunks int) (DownloadFileResult, error) {
	meta, resumed, err := loadOrCreateDownloadFileMeta(rawURL, remote, partPath, metaPath, chunks)
	if err != nil {
		return DownloadFileResult{}, err
	}
	file, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return DownloadFileResult{}, err
	}
	if err := file.Truncate(remote.Size); err != nil {
		_ = file.Close()
		return DownloadFileResult{}, err
	}

	bar := downloadProgressWriter(getbar, remote.Size)
	progressWriter, closeProgress := concurrentProgressWriter(bar)
	var metaMu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, len(meta.Chunks))
	for _, chunk := range meta.Chunks {
		if chunk.Done {
			continue
		}
		chunk := chunk
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := downloadFileChunk(rawURL, file, chunk, progressWriter, opts); err != nil {
				errCh <- err
				return
			}
			metaMu.Lock()
			for i := range meta.Chunks {
				if meta.Chunks[i].Start == chunk.Start && meta.Chunks[i].End == chunk.End {
					meta.Chunks[i].Done = true
					break
				}
			}
			meta.UpdatedAt = time.Now()
			saveErr := saveDownloadFileMeta(metaPath, meta)
			metaMu.Unlock()
			if saveErr != nil {
				errCh <- saveErr
			}
		}()
	}
	wg.Wait()
	closeProgress()
	closeErr := file.Close()
	close(errCh)
	restartSingle := false
	for err := range errCh {
		if err != nil {
			if errors.Is(err, errDownloadRangeReturnedOK) {
				restartSingle = true
				continue
			}
			return DownloadFileResult{}, err
		}
	}
	if closeErr != nil {
		return DownloadFileResult{}, closeErr
	}
	if restartSingle {
		_ = os.Remove(partPath)
		_ = os.Remove(metaPath)
		return downloadFileSingle(rawURL, target, partPath, metaPath, getbar, opts)
	}
	if fileSize(partPath) != remote.Size {
		return DownloadFileResult{}, fmt.Errorf("download size mismatch: expected %d, got %d", remote.Size, fileSize(partPath))
	}
	if err := replaceFile(partPath, target); err != nil {
		return DownloadFileResult{}, err
	}
	_ = os.Remove(metaPath)
	if finisher, ok := bar.(progressFinisher); ok {
		finisher.Finish()
	}
	return DownloadFileResult{Path: target, Resumed: resumed, Parallel: true, Size: remote.Size, ETag: remote.ETag, LastModified: remote.LastModified}, nil
}

func loadOrCreateDownloadFileMeta(rawURL string, remote downloadFileRemote, partPath, metaPath string, chunks int) (downloadFileMeta, bool, error) {
	if meta, ok := loadDownloadFileMeta(metaPath); ok && metaMatchesDownloadFile(meta, rawURL, remote, chunks) && fileSize(partPath) > 0 {
		return meta, true, nil
	}
	_ = os.Remove(partPath)
	meta := downloadFileMeta{
		Schema:       2,
		URL:          rawURL,
		Size:         remote.Size,
		ETag:         remote.ETag,
		LastModified: remote.LastModified,
		ChunkSize:    minChunkSize,
		Chunks:       planDownloadChunks(remote.Size, chunks),
		UpdatedAt:    time.Now(),
	}
	return meta, false, saveDownloadFileMeta(metaPath, meta)
}

func metaMatchesDownloadFile(meta downloadFileMeta, rawURL string, remote downloadFileRemote, chunks int) bool {
	if meta.Schema != 2 || meta.URL != rawURL || meta.Size != remote.Size || len(meta.Chunks) != chunks {
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

func downloadFileChunk(rawURL string, file *os.File, chunk downloadChunkMeta, progressWriter io.Writer, opts Options) error {
	rangeHeader := fmt.Sprintf("bytes=%d-%d", chunk.Start, chunk.End)
	resp, err := requestWithOptions(http.MethodGet, rawURL, rangeHeader, opts)
	if err != nil {
		return fmt.Errorf("download range %s: %w", rangeHeader, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == http.StatusOK {
			return fmt.Errorf("download range %s: %w", rangeHeader, errDownloadRangeReturnedOK)
		}
		return fmt.Errorf("download range %s returned status %d", rangeHeader, resp.StatusCode)
	}
	buf := make([]byte, 256*1024)
	offset := chunk.Start
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.WriteAt(buf[:n], offset); err != nil {
				return fmt.Errorf("download range %s: %w", rangeHeader, err)
			}
			if progressWriter != nil {
				_, _ = progressWriter.Write(buf[:n])
			}
			offset += int64(n)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("download range %s: %w", rangeHeader, readErr)
		}
	}
	if offset != chunk.End+1 {
		return fmt.Errorf("download range %s length mismatch", rangeHeader)
	}
	return nil
}

func effectiveFileChunkCount(requested int, size int64) int {
	return effectiveChunkCount(requested, size)
}

func planDownloadChunks(size int64, chunks int) []downloadChunkMeta {
	ranges := splitByteRanges(size, chunks)
	metas := make([]downloadChunkMeta, 0, len(ranges))
	for _, br := range ranges {
		metas = append(metas, downloadChunkMeta{Start: br.Start, End: br.End})
	}
	return metas
}

func probeDownloadFile(rawURL string, opts Options) (downloadFileRemote, bool) {
	resp, err := requestWithOptions(http.MethodHead, rawURL, "", opts)
	if err != nil {
		verbosef("download file probe failed: %v", err)
		return downloadFileRemote{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		verbosef("download file probe unsupported status: %s", resp.Status)
		return downloadFileRemote{}, false
	}
	size := resp.ContentLength
	if size <= 0 {
		size, _ = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	}
	return downloadFileRemote{
		Size:         size,
		AcceptRange:  strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, size > 0
}

func canResumeDownloadFile(rawURL string, partSize int64, metaPath string, remote downloadFileRemote) bool {
	if partSize <= 0 || !remote.AcceptRange || remote.Size <= 0 || partSize > remote.Size {
		return false
	}
	meta, ok := loadDownloadFileMeta(metaPath)
	if !ok || meta.URL != rawURL || meta.Size != remote.Size {
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

func writeDownloadResponse(path string, body io.Reader, bar io.Writer, appendFile bool) error {
	flag := os.O_CREATE | os.O_WRONLY
	if appendFile {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	out, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(io.MultiWriter(out, bar), body)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func replaceFile(src, dst string) error {
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(src, dst)
}

func saveDownloadFileMeta(path string, meta downloadFileMeta) error {
	if meta.Schema == 0 {
		meta.Schema = 2
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func loadDownloadFileMeta(path string) (downloadFileMeta, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return downloadFileMeta{}, false
	}
	var meta downloadFileMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return downloadFileMeta{}, false
	}
	return meta, true
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}
