package install

import (
	"fmt"
	"path"
	"time"
)

type downloadBodyResult struct {
	Body    []byte
	ModTime time.Time
}

func shouldApplyDownloadedModTime(file ExtractedFile, assetURL string, opts Options, modTime time.Time) bool {
	if modTime.IsZero() || opts.ExtractFile != "" || opts.All {
		return false
	}
	return file.ArchiveName == path.Base(assetURL)
}

func extractAllTo(extractor DirectAllExtractor, body []byte, output string, stripComponents int) ([]string, error) {
	if withOptions, ok := extractor.(directAllExtractorWithOptions); ok {
		return withOptions.ExtractAllToWithOptions(body, output, ArchiveExtractOptions{StripComponents: stripComponents})
	}
	if stripComponents > 0 {
		return nil, fmt.Errorf("strip-components is not supported for this extractor")
	}
	return extractor.ExtractAllTo(body, output)
}
