package install

import (
	"io"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestAutoSelectExtractedFileByArch(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `arm64\WinDirStat.exe`, Name: `arm64\WinDirStat.exe`, mode: 0o666},
		{ArchiveName: `x86\WinDirStat.exe`, Name: `x86\WinDirStat.exe`, mode: 0o666},
		{ArchiveName: `x64\WinDirStat.exe`, Name: `x64\WinDirStat.exe`, mode: 0o666},
	}

	selected, ok := autoSelectExtractedFile(candidates, "windows", "amd64")
	if !ok {
		t.Fatal("expected auto selection for amd64 candidates")
	}
	if selected.ArchiveName != `x64\WinDirStat.exe` {
		t.Fatalf("expected x64 executable to be selected, got %q", selected.ArchiveName)
	}
}

func TestAutoSelectExtractedFilePicksOnlyWindowsExe(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `LICENSE`, Name: `LICENSE`, mode: 0o666},
		{ArchiveName: `gsa.exe`, Name: `gsa.exe`, mode: 0o666},
	}

	selected, ok := autoSelectExtractedFile(candidates, "windows", "amd64")
	if !ok {
		t.Fatal("expected auto selection for the only Windows executable")
	}
	if selected.ArchiveName != `gsa.exe` {
		t.Fatalf("expected gsa.exe to be selected, got %q", selected.ArchiveName)
	}
}

func TestAutoSelectExtractedFileKeepsPromptForMultipleWindowsExe(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `gsa.exe`, Name: `gsa.exe`, mode: 0o666},
		{ArchiveName: `gsa-helper.exe`, Name: `gsa-helper.exe`, mode: 0o666},
	}

	_, ok := autoSelectExtractedFile(candidates, "windows", "amd64")
	if ok {
		t.Fatal("expected multiple Windows executables to keep prompt fallback")
	}
}

func TestResolveExtractedFileUsesExplicitSystemForWindowsExeSelection(t *testing.T) {
	runner := &InstallRunner{Stderr: io.Discard}
	candidates := []ExtractedFile{
		{ArchiveName: `LICENSE`, Name: `LICENSE`, mode: 0o666},
		{ArchiveName: `gsa.exe`, Name: `gsa.exe`, mode: 0o666},
	}

	selected, all, err := runner.resolveExtractedFile(candidates, Options{System: "windows/amd64"})
	if err != nil {
		t.Fatalf("resolve extracted file: %v", err)
	}
	if all {
		t.Fatal("expected single file selection, got extract-all")
	}
	if selected.ArchiveName != `gsa.exe` {
		t.Fatalf("expected gsa.exe to be selected, got %q", selected.ArchiveName)
	}
}

func TestAutoSelectExtractedFileKeepsPromptWhenAmbiguous(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `bin\tool.exe`, Name: `bin\tool.exe`, mode: 0o666},
		{ArchiveName: `tools\tool.exe`, Name: `tools\tool.exe`, mode: 0o666},
	}

	_, ok := autoSelectExtractedFile(candidates, "windows", "amd64")
	if ok {
		t.Fatal("expected ambiguous candidates to keep prompt fallback")
	}
}

func TestAutoExtractCurrentPlatformExecutablesFiltersOtherPlatformExecutables(t *testing.T) {
	candidates := []ExtractedFile{
		{ArchiveName: `x86_64\uv.exe`, Name: `x86_64\uv.exe`, mode: 0o666},
		{ArchiveName: `x86\uv.exe`, Name: `x86\uv.exe`, mode: 0o666},
		{ArchiveName: `linux\uv`, Name: `linux\uv`, mode: 0o755},
	}

	selected, ok := autoExtractCurrentPlatformExecutables(candidates, Options{System: "windows/amd64"})
	if !ok {
		t.Fatal("expected current-platform executable selection")
	}
	assert.Eq(t, 1, len(selected))
	assert.Eq(t, `x86_64\uv.exe`, selected[0].ArchiveName)
}
