package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var system7zCandidates = []string{"7z", "7zz", "7za"}

func resolveSystem7zPath(configured string) string {
	if configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured
		}
	}
	for _, name := range system7zCandidates {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func shouldUseSystem7z(filename string, extractAll bool) bool {
	name := strings.ToLower(filepath.Base(filename))
	switch {
	case strings.HasSuffix(name, ".7z"),
		strings.HasSuffix(name, ".rar"),
		strings.HasSuffix(name, ".msi"),
		strings.HasSuffix(name, ".cab"),
		strings.HasSuffix(name, ".iso"):
		return true
	case strings.HasSuffix(name, ".exe") && extractAll:
		return true
	default:
		return false
	}
}

type System7zExtractor struct {
	Filename string
	Tool     string
	Chooser  Chooser
	Exe      string
}

func NewSystem7zExtractor(filename, tool string, chooser Chooser, exe string) *System7zExtractor {
	return &System7zExtractor{Filename: filename, Tool: tool, Chooser: chooser, Exe: exe}
}

func (e *System7zExtractor) Extract(data []byte, multiple bool) (ExtractedFile, []ExtractedFile, error) {
	return ExtractedFile{}, nil, fmt.Errorf("system 7z extractor is not implemented")
}
