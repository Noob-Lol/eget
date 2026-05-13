package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestNewExtractorSupports7zArchives(t *testing.T) {
	extractor := NewExtractor("tool.7z", "tool", NewBinaryChooser("tool"))
	if _, ok := extractor.(*ArchiveExtractor); !ok {
		t.Fatalf("expected 7z extractor to use archive extractor, got %T", extractor)
	}
}

func TestExtractFileRequestsMultipleMatches(t *testing.T) {
	if extractAllFromFileSpec("README") {
		t.Fatal("expected single file spec to keep single extraction mode")
	}
	if !extractAllFromFileSpec("README,LICENSE") {
		t.Fatal("expected comma-separated file spec to enable multi extraction mode")
	}
	if !extractAllFromFileSpec("*.exe") {
		t.Fatal("expected glob file spec to enable multi extraction mode")
	}
}

func TestArchiveDirectoryExtractRejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create("safe/"); err != nil {
		t.Fatalf("create zip dir: %v", err)
	}
	w, err := zw.Create("safe/../../evil.txt")
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}
	if _, err := w.Write([]byte("evil")); err != nil {
		t.Fatalf("write zip file: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	chooser, err := NewFileChooser("*")
	if err != nil {
		t.Fatalf("NewFileChooser: %v", err)
	}
	extractor := NewArchiveExtractor(chooser, NewZipArchive, nil)
	file, _, err := extractor.Extract(buf.Bytes(), true)
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("expected unsafe archive path error, got %v", err)
	}

	root := t.TempDir()
	if file.Extract != nil {
		t.Fatal("expected no extract function for unsafe archive")
	}
	if _, statErr := os.Stat(filepath.Join(root, "evil.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("expected traversal output to be absent, stat error: %v", statErr)
	}
}

func TestArchiveExtractorExtractAllToPreservesEmptyDirsAndTimestamps(t *testing.T) {
	pluginTime := time.Date(2024, 1, 2, 3, 4, 4, 0, time.UTC)
	fileTime := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	dirHeader := &zip.FileHeader{Name: `Plugins\`, Method: zip.Store, Modified: pluginTime}
	dirHeader.SetMode(0o755 | os.ModeDir)
	if _, err := zw.CreateHeader(dirHeader); err != nil {
		t.Fatalf("create zip dir: %v", err)
	}
	fileHeader := &zip.FileHeader{Name: "KeePass.exe", Method: zip.Store, Modified: fileTime}
	fileHeader.SetMode(0o644)
	w, err := zw.CreateHeader(fileHeader)
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}
	if _, err := w.Write([]byte("keepass")); err != nil {
		t.Fatalf("write zip file: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	chooser, err := NewFileChooser("*")
	if err != nil {
		t.Fatalf("NewFileChooser: %v", err)
	}
	extractor := NewArchiveExtractor(chooser, NewZipArchive, nil)
	root := t.TempDir()
	files, err := extractor.ExtractAllTo(buf.Bytes(), root)
	if err != nil {
		t.Fatalf("extract all: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one extracted file, got %#v", files)
	}

	dirInfo, err := os.Stat(filepath.Join(root, "Plugins"))
	if err != nil {
		t.Fatalf("expected empty Plugins dir: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Fatalf("expected Plugins to be a directory")
	}
	if !dirInfo.ModTime().Equal(pluginTime) {
		t.Fatalf("expected Plugins mtime %s, got %s", pluginTime, dirInfo.ModTime())
	}
	fileInfo, err := os.Stat(filepath.Join(root, "KeePass.exe"))
	if err != nil {
		t.Fatalf("expected KeePass.exe: %v", err)
	}
	if !fileInfo.ModTime().Equal(fileTime) {
		t.Fatalf("expected KeePass.exe mtime %s, got %s", fileTime, fileInfo.ModTime())
	}
}

func TestArchiveExtractorExtractPreservesFileTimestamp(t *testing.T) {
	fileTime := time.Date(2024, 3, 4, 5, 6, 8, 0, time.UTC)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fileHeader := &zip.FileHeader{Name: "bin/tool.exe", Method: zip.Store, Modified: fileTime}
	fileHeader.SetMode(0o755)
	w, err := zw.CreateHeader(fileHeader)
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}
	if _, err := w.Write([]byte("tool")); err != nil {
		t.Fatalf("write zip file: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	extractor := NewArchiveExtractor(NewBinaryChooser("tool"), NewZipArchive, nil)
	file, candidates, err := extractor.Extract(buf.Bytes(), false)
	if err != nil {
		t.Fatalf("extract candidate: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected direct file, got candidates %#v", candidates)
	}
	target := filepath.Join(t.TempDir(), "tool.exe")
	if err := file.Extract(target); err != nil {
		t.Fatalf("extract file: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat extracted file: %v", err)
	}
	if !info.ModTime().Equal(fileTime) {
		t.Fatalf("expected tool.exe mtime %s, got %s", fileTime, info.ModTime())
	}
}

type streamArchiveEntry struct {
	file    File
	content string
}

type streamArchive struct {
	entries []streamArchiveEntry
	idx     int
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

func TestArchiveExtractorExtractAllToStreamsArchiveOnce(t *testing.T) {
	opens := 0
	extractor := NewArchiveExtractor(&GlobChooser{all: true, expr: "*"}, func(data []byte, d DecompFn) (Archive, error) {
		opens++
		return &streamArchive{entries: []streamArchiveEntry{
			{file: File{Name: "docs", Mode: 0o755, Type: TypeDir}},
			{file: File{Name: "docs/readme.txt", Mode: 0o644, Type: TypeNormal}, content: "readme"},
			{file: File{Name: "bin/tool.exe", Mode: 0o755, Type: TypeNormal}, content: "tool"},
		}}, nil
	}, nil)

	tmp := t.TempDir()
	files, err := extractor.ExtractAllTo([]byte("archive"), tmp)
	if err != nil {
		t.Fatalf("extract all: %v", err)
	}

	if opens != 1 {
		t.Fatalf("expected archive to be opened once, got %d", opens)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 extracted files, got %#v", files)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "docs", "readme.txt"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(data) != "readme" {
		t.Fatalf("expected readme content, got %q", string(data))
	}
	data, err = os.ReadFile(filepath.Join(tmp, "bin", "tool.exe"))
	if err != nil {
		t.Fatalf("read extracted executable: %v", err)
	}
	if string(data) != "tool" {
		t.Fatalf("expected tool content, got %q", string(data))
	}
}

func TestTarArchiveNextRejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "../evil.txt", Mode: 0o644, Size: int64(len("evil"))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatalf("write tar file: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	ar, err := NewTarArchive(buf.Bytes(), func(r io.Reader) (io.Reader, error) { return r, nil })
	if err != nil {
		t.Fatalf("NewTarArchive: %v", err)
	}
	_, err = ar.Next()
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("expected unsafe archive path error, got %v", err)
	}
}

func TestZipArchiveNextRejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../evil.txt")
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}
	if _, err := w.Write([]byte("evil")); err != nil {
		t.Fatalf("write zip file: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	ar, err := NewZipArchive(buf.Bytes(), nil)
	if err != nil {
		t.Fatalf("NewZipArchive: %v", err)
	}
	_, err = ar.Next()
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("expected unsafe archive path error, got %v", err)
	}
}

func TestAssetDetectorSupportsRegexInclude(t *testing.T) {
	re, err := compileAssetRegex(`\.deb$`)
	if err != nil {
		t.Fatalf("compileAssetRegex: %v", err)
	}
	d := &assetDetector{Asset: `\.deb$`, Regex: re}

	got, candidates, err := d.Detect([]string{
		"https://example.com/pkg_1.0.0_amd64.deb",
		"https://example.com/pkg_1.0.0_amd64.rpm",
	})
	if err != nil {
		t.Fatalf("Detect(): %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", candidates)
	}
	if got != "https://example.com/pkg_1.0.0_amd64.deb" {
		t.Fatalf("expected deb asset to match, got %q", got)
	}
}

func TestAssetDetectorSupportsRegexExclude(t *testing.T) {
	re, err := compileAssetRegex(`\.deb$`)
	if err != nil {
		t.Fatalf("compileAssetRegex: %v", err)
	}
	d := &assetDetector{Asset: `\.deb$`, Anti: true, Regex: re}

	got, candidates, err := d.Detect([]string{
		"https://example.com/pkg_1.0.0_amd64.deb",
		"https://example.com/pkg_1.0.0_amd64.rpm",
	})
	if err != nil {
		t.Fatalf("Detect(): %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", candidates)
	}
	if got != "https://example.com/pkg_1.0.0_amd64.rpm" {
		t.Fatalf("expected rpm asset to remain after exclude, got %q", got)
	}
}

func TestAssetDetectorMatchesPlainFilterCaseInsensitive(t *testing.T) {
	d := &assetDetector{Asset: "setup"}

	got, candidates, err := d.Detect([]string{
		"https://example.com/WinMerge-2.16.56-x64-Setup.exe",
		"https://example.com/WinMerge-2.16.56-x64.zip",
	})
	if err != nil {
		t.Fatalf("Detect(): %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", candidates)
	}
	if got != "https://example.com/WinMerge-2.16.56-x64-Setup.exe" {
		t.Fatalf("expected setup asset to match, got %q", got)
	}
}
