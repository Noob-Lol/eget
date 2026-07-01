package client

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
)

type progressFinisher interface {
	Finish(...string)
}

const (
	defaultAutoChunkConcurrency = 5
	minChunkSize                = 4 * 1024 * 1024
)

type byteRange struct {
	Start int64
	End   int64
}

func Download(rawURL string, out io.Writer, getbar func(size int64) io.Writer, opts Options) error {
	_, err := DownloadWithResult(rawURL, out, getbar, opts)
	return err
}

func DownloadWithResult(rawURL string, out io.Writer, getbar func(size int64) io.Writer, opts Options) (DownloadResult, error) {
	if isLocalFile(rawURL) {
		file, err := os.Open(rawURL)
		if err != nil {
			return DownloadResult{}, err
		}
		defer file.Close()
		_, err = io.Copy(out, file)
		return DownloadResult{}, err
	}

	restoreDownloadNotice := setDownloadNoticeURLForRequest(rawURL)
	defer restoreDownloadNotice()

	if opts.ChunkConcurrency != 1 {
		if remote, ok := probeDownloadFile(rawURL, opts); ok && remote.AcceptRange {
			chunks := effectiveChunkCount(opts.ChunkConcurrency, remote.Size)
			if chunks > 1 {
				bar := downloadProgressWriter(getbar, remote.Size)
				if err := downloadRangeChunks(rawURL, out, bar, remote.Size, chunks, opts); err != nil {
					return DownloadResult{}, err
				}
				return DownloadResult{LastModified: remote.LastModified, Filename: remote.Filename}, nil
			}
		}
	}

	resp, err := downloadGetWithOptions(rawURL, opts)
	if err != nil {
		return DownloadResult{}, err
	}
	defer resp.Body.Close()
	verbosef("download response bytes: %d", resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return DownloadResult{}, err
		}
		return DownloadResult{}, fmt.Errorf("download error: %d: %s", resp.StatusCode, body)
	}

	bar := downloadProgressWriter(getbar, resp.ContentLength)
	_, err = io.Copy(io.MultiWriter(out, bar), resp.Body)
	if err == nil {
		if finisher, ok := bar.(progressFinisher); ok {
			finisher.Finish()
		}
	}
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{LastModified: resp.Header.Get("Last-Modified"), Filename: responseFilename(resp, rawURL)}, nil
}

func effectiveChunkCount(requested int, size int64) int {
	if requested == 1 || size < 2*minChunkSize {
		return 1
	}
	maxBySize := int(size / minChunkSize)
	if maxBySize < 2 {
		return 1
	}
	limit := requested
	if limit <= 0 {
		limit = defaultAutoChunkConcurrency
	}
	if limit > maxBySize {
		limit = maxBySize
	}
	if limit < 2 {
		return 1
	}
	return limit
}

func splitByteRanges(size int64, chunks int) []byteRange {
	if chunks <= 1 {
		return []byteRange{{Start: 0, End: size - 1}}
	}
	ranges := make([]byteRange, 0, chunks)
	step := size / int64(chunks)
	start := int64(0)
	for i := 0; i < chunks; i++ {
		end := start + step - 1
		if i == chunks-1 {
			end = size - 1
		}
		ranges = append(ranges, byteRange{Start: start, End: end})
		start = end + 1
	}
	return ranges
}

func downloadRangeChunks(rawURL string, out io.Writer, bar io.Writer, size int64, chunks int, opts Options) error {
	if size <= 0 {
		return fmt.Errorf("invalid range download size %d", size)
	}
	bodyLen, err := intFromContentLength(size)
	if err != nil {
		return err
	}
	body := make([]byte, bodyLen)
	ranges := splitByteRanges(size, chunks)
	progressWriter, closeProgress := concurrentProgressWriter(bar)

	var wg sync.WaitGroup
	errCh := make(chan error, len(ranges))
	for _, br := range ranges {
		br := br
		wg.Add(1)
		go func() {
			defer wg.Done()
			rangeHeader := fmt.Sprintf("bytes=%d-%d", br.Start, br.End)
			resp, err := requestWithOptions(http.MethodGet, rawURL, rangeHeader, opts)
			if err != nil {
				errCh <- fmt.Errorf("download range %s: %w", rangeHeader, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusPartialContent {
				errCh <- fmt.Errorf("download range %s returned status %d", rangeHeader, resp.StatusCode)
				return
			}
			dst := body[br.Start : br.End+1]
			n, err := readRangeBody(resp.Body, dst, progressWriter)
			if err != nil {
				errCh <- fmt.Errorf("download range %s: %w", rangeHeader, err)
				return
			}
			if int64(n) != br.End-br.Start+1 {
				errCh <- fmt.Errorf("download range %s length mismatch", rangeHeader)
				return
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			closeProgress()
			return err
		}
	}
	closeProgress()
	_, err = out.Write(body)
	if err == nil {
		if finisher, ok := bar.(progressFinisher); ok {
			finisher.Finish()
		}
	}
	return err
}

func readRangeBody(body io.Reader, dst []byte, progressWriter io.Writer) (int, error) {
	const progressReadChunkSize = 256 * 1024
	total := 0
	for total < len(dst) {
		end := total + progressReadChunkSize
		if end > len(dst) {
			end = len(dst)
		}
		n, err := body.Read(dst[total:end])
		if n > 0 {
			if progressWriter != nil {
				_, _ = progressWriter.Write(dst[total : total+n])
			}
			total += n
		}
		if err != nil {
			if err == io.EOF && total == len(dst) {
				return total, nil
			}
			return total, err
		}
	}
	return total, nil
}

func intFromContentLength(size int64) (int, error) {
	if size <= 0 {
		return 0, fmt.Errorf("invalid range download size %d", size)
	}
	maxInt := int64(^uint(0) >> 1)
	if size > maxInt {
		return 0, fmt.Errorf("range download size %d is too large for this platform", size)
	}
	return int(size), nil
}
