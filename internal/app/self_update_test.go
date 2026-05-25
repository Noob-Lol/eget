package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	"github.com/inherelab/eget/internal/install"
)

func TestSelfUpdateSkipsWhenLatestMatchesCurrentVersion(t *testing.T) {
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			assert.Eq(t, "inherelab/eget", target.Repo)
			return LatestInfo{Tag: "v1.7.1"}, nil
		},
	}

	result, err := svc.Update(SelfUpdateOptions{CheckOnly: true})

	assert.NoErr(t, err)
	assert.False(t, result.Updated)
	assert.False(t, result.Outdated)
	assert.Eq(t, "v1.7.1", result.LatestVersion)
}

func TestSelfUpdateReportsOutdatedWhenLatestDiffers(t *testing.T) {
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			return LatestInfo{Tag: "v1.7.2"}, nil
		},
	}

	result, err := svc.Update(SelfUpdateOptions{CheckOnly: true})

	assert.NoErr(t, err)
	assert.True(t, result.Outdated)
	assert.Eq(t, "1.7.1", result.CurrentVersion)
	assert.Eq(t, "v1.7.2", result.LatestVersion)
}

func TestSelfUpdateDownloadsExpectedPlatformAsset(t *testing.T) {
	replacement := filepath.Join(t.TempDir(), "eget")
	assert.NoErr(t, os.WriteFile(replacement, []byte("new"), 0o755))
	installer := &fakeSelfUpdateInstaller{
		result: RunResult{ExtractedFiles: []string{replacement}},
	}
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			return LatestInfo{Tag: "v1.7.2"}, nil
		},
		Installer:     installer,
		RuntimeGOOS:   "linux",
		RuntimeGOARCH: "amd64",
		ExecutablePath: func() (string, error) {
			return filepath.Join(t.TempDir(), "eget"), nil
		},
		Replacer: fakeSelfReplacer{},
	}

	_, err := svc.Update(SelfUpdateOptions{})

	assert.NoErr(t, err)
	assert.Eq(t, SelfUpdateRepo, installer.target)
	assert.Eq(t, "linux/amd64", installer.opts.System)
	assert.Eq(t, "eget-linux-amd64", installer.opts.ExtractFile)
	assert.Eq(t, []string{"PRE:eget-", "linux-amd64", "SUF:.zip"}, installer.opts.Asset)
}

func TestSelfUpdateExtractFileUsesExeOnWindows(t *testing.T) {
	assert.Eq(t, "eget-windows-amd64.exe", selfUpdateExtractFile("windows", "amd64"))
}

func TestSelfUpdateReplacesExecutable(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "eget")
	replacement := filepath.Join(dir, "download", "eget")
	assert.NoErr(t, os.WriteFile(current, []byte("old"), 0o755))
	assert.NoErr(t, os.MkdirAll(filepath.Dir(replacement), 0o755))
	assert.NoErr(t, os.WriteFile(replacement, []byte("new"), 0o755))
	installer := &fakeSelfUpdateInstaller{
		result: RunResult{ExtractedFiles: []string{replacement}},
	}
	replacer := &recordingSelfReplacer{}
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			return LatestInfo{Tag: "v1.7.2"}, nil
		},
		Installer:      installer,
		Replacer:       replacer,
		RuntimeGOOS:    "linux",
		RuntimeGOARCH:  "amd64",
		ExecutablePath: func() (string, error) { return current, nil },
	}

	result, err := svc.Update(SelfUpdateOptions{})

	assert.NoErr(t, err)
	assert.True(t, result.Updated)
	assert.Eq(t, current, result.Executable)
	assert.Eq(t, replacement, result.Replacement)
	assert.Eq(t, current, replacer.current)
	assert.Eq(t, replacement, replacer.replacement)
}

func TestSelfUpdateRejectsUnexpectedReplacementName(t *testing.T) {
	replacement := filepath.Join(t.TempDir(), "wrong")
	assert.NoErr(t, os.WriteFile(replacement, []byte("new"), 0o755))
	installer := &fakeSelfUpdateInstaller{
		result: RunResult{ExtractedFiles: []string{replacement}},
	}
	svc := SelfUpdateService{
		CurrentVersion: "1.7.1",
		LatestInfo: func(target LatestCheckTarget) (LatestInfo, error) {
			return LatestInfo{Tag: "v1.7.2"}, nil
		},
		Installer:     installer,
		Replacer:      fakeSelfReplacer{},
		RuntimeGOOS:   "linux",
		RuntimeGOARCH: "amd64",
	}

	_, err := svc.Update(SelfUpdateOptions{})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "replacement must be eget")
}

type fakeSelfUpdateInstaller struct {
	target string
	opts   install.Options
	result RunResult
}

func (f *fakeSelfUpdateInstaller) DownloadTarget(target string, opts install.Options) (RunResult, error) {
	f.target = target
	f.opts = opts
	return f.result, nil
}

type fakeSelfReplacer struct{}

func (fakeSelfReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	return SelfReplaceResult{}, nil
}

type recordingSelfReplacer struct {
	current     string
	replacement string
}

func (r *recordingSelfReplacer) Replace(currentPath, replacementPath string) (SelfReplaceResult, error) {
	r.current = currentPath
	r.replacement = replacementPath
	return SelfReplaceResult{}, nil
}
