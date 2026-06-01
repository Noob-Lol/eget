package install

import (
	"fmt"
	"io"
	"testing"
)

func TestNewFileChooserSupportsCommaSeparatedPatterns(t *testing.T) {
	chooser, err := NewFileChooser("README*, LICENSE")
	if err != nil {
		t.Fatalf("NewFileChooser(): %v", err)
	}

	if direct, possible := chooser.Choose("README.md", false, 0); direct || !possible {
		t.Fatalf("expected README.md to match, got direct=%t possible=%t", direct, possible)
	}
	if direct, possible := chooser.Choose("docs/LICENSE", false, 0); direct || !possible {
		t.Fatalf("expected docs/LICENSE to match, got direct=%t possible=%t", direct, possible)
	}
	if direct, possible := chooser.Choose("bin/tool.exe", false, 0); direct || possible {
		t.Fatalf("expected bin/tool.exe to be ignored, got direct=%t possible=%t", direct, possible)
	}
}

func (s *streamArchive) Next() (File, error) {
	if s.idx >= len(s.entries) {
		return File{}, io.EOF
	}
	file := s.entries[s.idx].file
	s.idx++
	return file, nil
}

func (s *streamArchive) ReadAll() ([]byte, error) {
	return nil, fmt.Errorf("ReadAll should not be used by direct extract-all")
}

func (s *streamArchive) WriteTo(w io.Writer) (int64, error) {
	entry := s.entries[s.idx-1]
	n, err := io.WriteString(w, entry.content)
	return int64(n), err
}
