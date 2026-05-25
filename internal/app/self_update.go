package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/inherelab/eget/internal/install"
)

const SelfUpdateRepo = "inherelab/eget"

type SelfUpdateInstaller interface {
	DownloadTarget(target string, opts install.Options) (RunResult, error)
}

type SelfUpdateOptions struct {
	CheckOnly bool
	Tag       string
	Asset     []string
	Install   install.Options
}

type SelfUpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	Outdated       bool
	Replacement    string
	Executable     string
	Deferred       bool
}

type SelfUpdateService struct {
	CurrentVersion string
	LatestInfo     LatestInfoFunc
	Installer      SelfUpdateInstaller
	Replacer       ExecutableReplacer
	RuntimeGOOS    string
	RuntimeGOARCH  string
	TempDir        func() (string, error)
	ExecutablePath func() (string, error)
}

func (s SelfUpdateService) Update(opts SelfUpdateOptions) (SelfUpdateResult, error) {
	result, err := s.versionResult(opts)
	if err != nil {
		return SelfUpdateResult{}, err
	}
	if opts.CheckOnly || !result.Outdated {
		return result, nil
	}
	if s.Installer == nil {
		return SelfUpdateResult{}, fmt.Errorf("self update installer is required")
	}

	goos, goarch := s.runtimePlatform()
	installOpts := opts.Install
	output, err := s.tempDir()
	if err != nil {
		return SelfUpdateResult{}, err
	}
	installOpts.Tag = firstNonEmpty(opts.Tag, installOpts.Tag)
	installOpts.Name = selfUpdateDownloadName(goos)
	installOpts.System = selfUpdateSystem(goos, goarch)
	installOpts.Output = output
	installOpts.OutputExplicit = false
	installOpts.DownloadOnly = true
	installOpts.ExtractFile = ""
	installOpts.Asset = selfUpdateAssetFilters()
	if len(opts.Asset) > 0 {
		installOpts.Asset = append([]string(nil), opts.Asset...)
	}

	downloaded, err := s.Installer.DownloadTarget(SelfUpdateRepo, installOpts)
	if err != nil {
		return SelfUpdateResult{}, err
	}
	replacement, err := singleSelfUpdateReplacement(downloaded, selfUpdateExecutableName(goos))
	if err != nil {
		return SelfUpdateResult{}, err
	}
	executable, err := s.executablePath()
	if err != nil {
		return SelfUpdateResult{}, err
	}
	replaceResult, err := s.replacer().Replace(executable, replacement)
	if err != nil {
		return SelfUpdateResult{}, err
	}

	result.Replacement = replacement
	result.Executable = executable
	result.Deferred = replaceResult.Deferred
	result.Updated = true
	return result, nil
}

func (s SelfUpdateService) versionResult(opts SelfUpdateOptions) (SelfUpdateResult, error) {
	result := SelfUpdateResult{CurrentVersion: s.CurrentVersion}
	current := normalizeSelfVersion(s.CurrentVersion)
	if opts.Tag != "" {
		result.LatestVersion = opts.Tag
		result.Outdated = true
		return result, nil
	}
	if s.LatestInfo == nil {
		return SelfUpdateResult{}, fmt.Errorf("latest info checker is required")
	}
	latest, err := s.LatestInfo(LatestCheckTarget{Repo: SelfUpdateRepo})
	if err != nil {
		return SelfUpdateResult{}, err
	}
	result.LatestVersion = latest.Tag
	result.Outdated = selfVersionOutdated(current, latest.Tag)
	return result, nil
}

func selfVersionOutdated(current, latest string) bool {
	current = normalizeSelfVersion(current)
	latest = normalizeSelfVersion(latest)
	if current == latest {
		return false
	}
	if base, ok := gitDescribeBaseVersion(current); ok {
		current = base
	}
	currentVersion, currentOK := parseSelfSemver(current)
	latestVersion, latestOK := parseSelfSemver(latest)
	if currentOK && latestOK {
		return currentVersion.lessThan(latestVersion)
	}
	return current != latest
}

func normalizeSelfVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	return value
}

func gitDescribeBaseVersion(value string) (string, bool) {
	parts := strings.Split(value, "-")
	if len(parts) < 3 {
		return "", false
	}
	if _, ok := parseSelfSemver(parts[0]); !ok {
		return "", false
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return "", false
	}
	if !strings.HasPrefix(parts[2], "g") || len(parts[2]) <= 1 {
		return "", false
	}
	return parts[0], true
}

type selfSemver struct {
	major int
	minor int
	patch int
}

func parseSelfSemver(value string) (selfSemver, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return selfSemver{}, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return selfSemver{}, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return selfSemver{}, false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return selfSemver{}, false
	}
	return selfSemver{major: major, minor: minor, patch: patch}, true
}

func (v selfSemver) lessThan(other selfSemver) bool {
	if v.major != other.major {
		return v.major < other.major
	}
	if v.minor != other.minor {
		return v.minor < other.minor
	}
	return v.patch < other.patch
}

func (s SelfUpdateService) runtimePlatform() (string, string) {
	goos := s.RuntimeGOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := s.RuntimeGOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return goos, goarch
}

func selfUpdateSystem(goos, goarch string) string {
	return goos + "/" + goarch
}

func selfUpdateAssetFilters() []string {
	return []string{"PRE:eget-"}
}

func selfUpdateDownloadName(goos string) string {
	if goos == "windows" {
		return "eget.exe"
	}
	return "eget"
}

func selfUpdateExecutableName(goos string) string {
	if goos == "windows" {
		return "eget.exe"
	}
	return "eget"
}

func singleSelfUpdateReplacement(result RunResult, expectedName string) (string, error) {
	if len(result.ExtractedFiles) != 1 {
		return "", fmt.Errorf("self update expected exactly one extracted file, got %d", len(result.ExtractedFiles))
	}
	replacement := result.ExtractedFiles[0]
	info, err := os.Stat(replacement)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("self update replacement is a directory: %s", replacement)
	}
	if filepath.Base(replacement) != expectedName {
		return "", fmt.Errorf("self update replacement must be %s, got %s", expectedName, filepath.Base(replacement))
	}
	return replacement, nil
}

func (s SelfUpdateService) executablePath() (string, error) {
	if s.ExecutablePath != nil {
		return s.ExecutablePath()
	}
	return os.Executable()
}

func (s SelfUpdateService) tempDir() (string, error) {
	if s.TempDir != nil {
		return s.TempDir()
	}
	return os.MkdirTemp("", "eget-self-update-*")
}

func (s SelfUpdateService) replacer() ExecutableReplacer {
	if s.Replacer != nil {
		return s.Replacer
	}
	return DefaultExecutableReplacer{}
}
