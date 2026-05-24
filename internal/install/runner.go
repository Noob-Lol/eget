package install

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/gookit/cliui/progress"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/gookit/goutil/x/termenv"
	storepkg "github.com/inherelab/eget/internal/installed"
	"github.com/inherelab/eget/internal/source/urltemplate"
	"github.com/inherelab/eget/internal/util"
)

const downloadProgressRedrawFreq = 256 * 1024

type RunResult struct {
	URL            string
	Tool           string
	Asset          string
	ExtractedFiles []string
	IsGUI          bool
	InstallMode    string
	InstallerFile  string
	Version        string
}

type Runner interface {
	Run(target string, opts Options) (RunResult, error)
}

type PromptFunc func(title, filterPrompt string, choices []string) (int, error)
type ConfirmFunc func(file string) (bool, error)

type InstallRunner struct {
	Service                *Service
	InstalledLoad          func() (map[string]string, map[string]string, error)
	Prompt                 PromptFunc
	ConfirmLaunchInstaller ConfirmFunc
	InstallerLauncher      InstallerLauncher
	AssetRunner            func(path string, args []string, stdout, stderr io.Writer) error
	Stdout                 io.Writer
	Stderr                 io.Writer
}

type downloadBodyResult struct {
	Body    []byte
	ModTime time.Time
}

type directAllExtractorWithOptions interface {
	ExtractAllToWithOptions([]byte, string, ArchiveExtractOptions) ([]string, error)
}

func NewRunner(service *Service) *InstallRunner {
	return &InstallRunner{
		Service:                service,
		ConfirmLaunchInstaller: defaultConfirmLaunchInstaller,
		InstallerLauncher:      DefaultInstallerLauncher{},
		Stdout:                 os.Stdout,
		Stderr:                 os.Stderr,
	}
}

func (r *InstallRunner) Run(target string, opts Options) (RunResult, error) {
	if r.Service == nil {
		return RunResult{}, fmt.Errorf("install service is required")
	}

	output := r.Stdout
	if output == nil {
		output = io.Discard
	}
	if opts.Quiet {
		output = io.Discard
	}

	finder, tool, err := r.Service.SelectFinder(target, &opts)
	if err != nil {
		return RunResult{}, err
	}
	if opts.CacheName == "" {
		opts.CacheName = tool
	}
	targetKind := DetectTargetKind(target)
	ccolor.Fprintf(output, "🚀 Install <info>%s</> from <info>%s</>\n", target, targetKind)
	// verbosef("target kind: %s", targetKind)
	assets, err := finder.Find()
	if err != nil {
		return RunResult{}, err
	}
	if templateFinder, ok := finder.(*urltemplate.Finder); ok {
		opts.URLTemplate.ResolvedVersion = templateFinder.Version
		opts.URLTemplate.ResolvedVars = templateFinder.Vars()
		opts.CacheVersion = templateFinder.Version
	}
	verbosef("finder assets: %d", len(assets))

	detector, err := r.Service.SelectDetector(&opts)
	if err != nil {
		return RunResult{}, err
	}

	url, candidates, err := detector.Detect(assets)
	if len(candidates) != 0 && err != nil {
		url, err = r.resolveCandidate(target, candidates, opts)
		if err != nil {
			return RunResult{}, err
		}
	} else if len(candidates) == 0 && err != nil {
		fallbackURL, fallbackAssets, fallbackErr := r.resolveVersionFallback(finder, detector, opts, err)
		if fallbackErr != nil {
			return RunResult{}, fallbackErr
		}
		if fallbackURL == "" {
			return RunResult{}, err
		}
		url = fallbackURL
		assets = fallbackAssets
	} else if err != nil {
		return RunResult{}, err
	}
	verbosef("selected asset url: %s", url)

	if _, err := fmt.Fprintf(output, "📦 Asset %s\n", url); err != nil {
		return RunResult{}, err
	}
	if err := validateInstallAction(opts); err != nil {
		return RunResult{}, err
	}

	downloaded, err := r.downloadBody(url, opts)
	if err != nil {
		return RunResult{}, fmt.Errorf("%s (URL: %s)", err, url)
	}

	sumAsset := checksumAsset(url, assets)

	verifier, err := r.Service.SelectVerifier(sumAsset, &opts)
	if err != nil {
		return RunResult{}, err
	}
	verbosef("verifier: checksum_asset=%t verify_arg=%t", sumAsset != "", opts.Verify != "")
	if err := verifier.Verify(downloaded.Body); err != nil {
		return RunResult{}, err
	}
	if opts.Verify == "" && sumAsset != "" {
		if _, err := fmt.Fprintf(output, "Checksum verified with %s\n", path.Base(sumAsset)); err != nil {
			return RunResult{}, err
		}
	} else if opts.Verify != "" {
		ccolor.Fprintln(output, "<error>Checksum verified</>")
	}

	if opts.DownloadOnly && opts.ExtractFile == "" && !opts.All {
		return r.extractDownloadedBody(url, tool, downloaded, opts, output)
	}

	if opts.URLTemplate.InstallAction == InstallActionRunAsset {
		assetPath, err := r.materializeRunAsset(downloaded.Body, url, opts)
		if err != nil {
			return RunResult{}, err
		}
		ccolor.Fprintf(output, "Running installer asset: <cyan>%s</>\n", assetPath)
		if err := r.runAsset(assetPath, opts.URLTemplate.InstallArgs); err != nil {
			return RunResult{}, err
		}
		return RunResult{
			URL:         url,
			Tool:        tool,
			Asset:       path.Base(url),
			InstallMode: InstallModeRunAsset,
			Version:     opts.URLTemplate.ResolvedVersion,
		}, nil
	}

	return r.extractDownloadedBody(url, tool, downloaded, opts, output)
}

func (r *InstallRunner) extractDownloadedBody(url, tool string, downloaded downloadBodyResult, opts Options, output io.Writer) (RunResult, error) {
	extractor, err := SelectExtractorAs[Extractor](r.Service, url, tool, &opts)
	if err != nil {
		return RunResult{}, err
	}
	verbosef("extractor selected for tool=%s", tool)

	if opts.All && opts.ExtractFile == "" && len(opts.RenameFiles) == 0 {
		if direct, ok := extractor.(DirectAllExtractor); ok && effectiveOutput(opts) != "-" {
			result := RunResult{
				URL:         url,
				Tool:        tool,
				Asset:       path.Base(url),
				IsGUI:       opts.IsGUI,
				InstallMode: opts.InstallMode,
			}
			paths, err := extractAllTo(direct, downloaded.Body, effectiveOutput(opts), opts.StripComponents)
			if err != nil {
				return RunResult{}, err
			}
			for _, out := range paths {
				verbosef("extract output: %s", out)
				ccolor.Fprintf(output, "✅ Extracted <cyan>%s</>\n", out)
				if out != "-" {
					result.ExtractedFiles = append(result.ExtractedFiles, out)
				}
			}
			return result, nil
		}
	}

	bin, bins, err := extractor.Extract(downloaded.Body, opts.All)
	if len(bins) != 0 && err != nil && !opts.All {
		bin, opts.All, err = r.resolveExtractedFile(bins, opts)
		if err != nil {
			return RunResult{}, err
		}
	} else if err != nil && len(bins) == 0 {
		return RunResult{}, err
	}
	if len(bins) == 0 {
		bins = []ExtractedFile{bin}
	}
	selectedName := selectedFileName(url, bin)
	if opts.IsGUI {
		opts.InstallMode = DetectGUIInstallMode(true, selectedName)
	} else if !opts.All && DetectInstallerKind(selectedName) != InstallerKindUnknown {
		confirmed, err := r.confirmLaunchInstaller(selectedName)
		if err != nil {
			return RunResult{}, err
		}
		if !confirmed {
			return RunResult{}, fmt.Errorf("installer launch cancelled")
		}
		opts.IsGUI = true
		opts.InstallMode = InstallModeInstaller
	}

	result := RunResult{
		URL:         url,
		Tool:        tool,
		Asset:       path.Base(url),
		IsGUI:       opts.IsGUI,
		InstallMode: opts.InstallMode,
		Version:     opts.URLTemplate.ResolvedVersion,
	}
	if opts.InstallMode == InstallModeInstaller {
		installerPath, err := r.materializeInstallerFile(downloaded.Body, url, bin, opts)
		if err != nil {
			return RunResult{}, err
		}
		result, err := r.launchGUIInstaller(installerPath, bin, opts)
		if err != nil {
			return RunResult{}, err
		}
		result.URL = url
		result.Tool = tool
		if result.Asset == "" {
			result.Asset = path.Base(url)
		}
		return result, nil
	}

	extract := func(file ExtractedFile) (string, error) {
		out, err := outputPath(file, effectiveOutput(opts), opts.All, opts.Name, opts.OutputExplicit, opts.RenameFiles)
		if err != nil {
			return "", err
		}
		if err := file.Extract(out); err != nil {
			return "", err
		}
		verbosef("extract output: %s", out)
		ccolor.Fprintf(output, "✅ Extracted <info>%s</> to <cyan>%s</>\n", file.ArchiveName, out)
		if out != "-" && !downloaded.ModTime.IsZero() {
			if err := applyModTime(out, downloaded.ModTime); err != nil {
				return "", err
			}
		}
		return out, nil
	}

	if opts.All {
		for _, file := range bins {
			out, err := extract(file)
			if err != nil {
				return RunResult{}, err
			}
			if out != "-" {
				result.ExtractedFiles = append(result.ExtractedFiles, out)
			}
		}
	} else {
		out, err := extract(bin)
		if err != nil {
			return RunResult{}, err
		}
		if out != "-" {
			result.ExtractedFiles = append(result.ExtractedFiles, out)
		}
	}

	return result, nil
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

func validateInstallAction(opts Options) error {
	action := opts.URLTemplate.InstallAction
	if action == "" {
		return nil
	}
	if action != InstallActionRunAsset {
		return fmt.Errorf("unsupported install_action %q", action)
	}
	if opts.Verify == "" && opts.URLTemplate.ChecksumURLTemplate == "" {
		return fmt.Errorf("install_action %q requires checksum source", action)
	}
	return nil
}

func (r *InstallRunner) materializeRunAsset(body []byte, url string, opts Options) (string, error) {
	if IsLocalFile(url) {
		return url, nil
	}
	target := CacheFilePathWithMeta(opts.CacheDir, url, CacheMeta{Name: opts.CacheName, Version: opts.CacheVersion})
	if target == "" {
		target = filepath.Join(os.TempDir(), filepath.Base(url))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	mode := os.FileMode(0o644)
	if runtime.GOOS != "windows" {
		mode = 0o755
	}
	if err := os.WriteFile(target, body, mode); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(target, 0o755)
	}
	return target, nil
}

func (r *InstallRunner) runAsset(path string, args []string) error {
	stdout, stderr := r.Stdout, r.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if r.AssetRunner != nil {
		return r.AssetRunner(path, append([]string(nil), args...), stdout, stderr)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (r *InstallRunner) resolveVersionFallback(finder Finder, detector Detector, opts Options, originalErr error) (string, []string, error) {
	if opts.FallbackVersions <= 0 || !isAssetSelectionMiss(originalErr) {
		return "", nil, nil
	}
	fallback, ok := finder.(VersionFallbackFinder)
	if !ok {
		return "", nil, nil
	}
	groups, err := fallback.FallbackVersionAssets(opts.FallbackVersions)
	if err != nil {
		return "", nil, err
	}
	for _, assets := range groups {
		url, candidates, err := detector.Detect(assets)
		if len(candidates) == 0 && err == nil {
			return url, assets, nil
		}
	}
	return "", nil, nil
}

func isAssetSelectionMiss(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "asset `") || msg == "no candidates found"
}

func selectedFileName(url string, file ExtractedFile) string {
	if file.ArchiveName != "" {
		return file.ArchiveName
	}
	if file.Name != "" {
		return file.Name
	}
	return path.Base(url)
}

func (r *InstallRunner) confirmLaunchInstaller(file string) (bool, error) {
	confirm := r.ConfirmLaunchInstaller
	if confirm == nil {
		confirm = defaultConfirmLaunchInstaller
	}
	return confirm(filepath.Base(file))
}

func defaultConfirmLaunchInstaller(file string) (bool, error) {
	fmt.Fprintf(os.Stderr, "%s looks like a GUI installer. Launch it now? [y/N]: ", file)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		if err == io.EOF && strings.TrimSpace(answer) == "" {
			return false, nil
		}
		if err != io.EOF {
			return false, err
		}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func effectiveOutput(opts Options) string {
	if opts.IsGUI && opts.InstallMode == InstallModePortable && !opts.OutputExplicit && opts.GuiTarget != "" {
		return opts.GuiTarget
	}
	return opts.Output
}

func (r *InstallRunner) launchGUIInstaller(path string, file ExtractedFile, opts Options) (RunResult, error) {
	kind := DetectInstallerKind(file.ArchiveName)
	if kind == InstallerKindUnknown {
		kind = DetectInstallerKind(file.Name)
	}
	launcher := r.InstallerLauncher
	if launcher == nil {
		launcher = DefaultInstallerLauncher{}
	}
	if err := launcher.LaunchInstaller(path, kind); err != nil {
		return RunResult{}, err
	}
	return RunResult{
		Asset:         filepath.Base(path),
		IsGUI:         true,
		InstallMode:   InstallModeInstaller,
		InstallerFile: path,
	}, nil
}

func (r *InstallRunner) materializeInstallerFile(body []byte, url string, file ExtractedFile, opts Options) (string, error) {
	if IsLocalFile(url) {
		return url, nil
	}
	cachePath := CacheFilePath(opts.CacheDir, url)
	if cachePath != "" && filepath.Base(cachePath) == filepath.Base(url) {
		if _, err := os.Stat(cachePath); err == nil {
			return cachePath, nil
		}
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(cachePath, body, 0o644); err != nil {
			return "", err
		}
		return cachePath, nil
	}

	target := installerMaterializePath(opts, file)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if file.Extract != nil {
		if err := file.Extract(target); err != nil {
			return "", err
		}
		return target, nil
	}
	if err := os.WriteFile(target, body, 0o755); err != nil {
		return "", err
	}
	return target, nil
}

func installerMaterializePath(opts Options, file ExtractedFile) string {
	dir := opts.CacheDir
	if dir == "" {
		dir = os.TempDir()
	}

	rawName := file.Name
	if rawName == "" {
		rawName = file.ArchiveName
	}

	name := "installer"
	if rawName != "" {
		if safeName, err := safeArchiveRelativePath(rawName); err == nil && safeName != "" {
			name = safeName
		}
	}

	return filepath.Join(dir, "installers", filepath.Base(name))
}

func (r *InstallRunner) downloadBody(url string, opts Options) (downloadBodyResult, error) {
	cachePath := CacheFilePathWithMeta(opts.CacheDir, url, CacheMeta{Name: opts.CacheName, Version: opts.CacheVersion})
	output := r.Stdout
	if output == nil || opts.Quiet {
		output = io.Discard
	}
	if cachePath != "" && !IsLocalFile(url) {
		if data, err := os.ReadFile(cachePath); err == nil {
			if !isInvalidCachedDownload(cachePath, data) {
				modTime := cachedDownloadModTime(url, cachePath, opts)
				ccolor.Fprintf(output, " - Using cached file <cyan>%s</>\n", filepath.Base(cachePath))
				return downloadBodyResult{Body: data, ModTime: modTime}, nil
			}
			verbosef("discard invalid cached archive: %s", cachePath)
		}
		result, err := DownloadFile(url, cachePath, r.downloadProgress(opts), opts)
		if err != nil {
			return downloadBodyResult{}, err
		}
		modTime := parseHTTPTime(result.LastModified)
		if !modTime.IsZero() {
			_ = applyModTime(cachePath, modTime)
		}
		data, err := os.ReadFile(cachePath)
		if err != nil {
			return downloadBodyResult{}, err
		}
		if modTime.IsZero() {
			modTime = fileModTime(cachePath)
		}
		return downloadBodyResult{Body: data, ModTime: modTime}, nil
	}

	buf := &bytes.Buffer{}
	err := Download(url, buf, r.downloadProgress(opts), opts)
	if err != nil {
		return downloadBodyResult{}, err
	}

	body := buf.Bytes()
	if cachePath != "" && !IsLocalFile(url) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
			_ = os.WriteFile(cachePath, body, 0o644)
		}
	}
	return downloadBodyResult{Body: body}, nil
}

func isInvalidCachedDownload(cachePath string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(cachePath))
	switch ext {
	case ".zip", ".gz", ".tgz", ".xz", ".bz2", ".zst", ".7z", ".rar":
	default:
		return false
	}
	trimmed := bytes.TrimSpace(data)
	lowerPrefix := strings.ToLower(string(trimmed[:min(len(trimmed), 64)]))
	return strings.HasPrefix(lowerPrefix, "<!doctype html") || strings.HasPrefix(lowerPrefix, "<html")
}

func cachedDownloadModTime(rawURL, cachePath string, opts Options) time.Time {
	if modTime := parseHTTPTime(ProbeLastModified(rawURL, opts)); !modTime.IsZero() {
		if err := applyModTime(cachePath, modTime); err != nil {
			verbosef("cached file modtime update failed: %v", err)
		}
		return modTime
	}
	return fileModTime(cachePath)
}

func parseHTTPTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := http.ParseTime(value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func (r *InstallRunner) downloadProgress(opts Options) func(int64) io.Writer {
	return func(size int64) io.Writer {
		pbout := r.Stdout
		if pbout == nil || opts.Quiet {
			pbout = io.Discard
		}
		return newDownloadProgress(pbout, size)
	}
}

func newDownloadProgress(out io.Writer, size int64) *progress.Progress {
	return NewDownloadProgress(out, size)
}

func NewDownloadProgress(out io.Writer, size int64) *progress.Progress {
	if out == nil {
		out = io.Discard
	}
	width, _ := termenv.GetTermSize()
	barWidth, format := downloadProgressLayout(width)
	p := progress.CustomBar(barWidth, progress.BarStyles[0], size)
	p.Out = out
	p.RedrawFreq = downloadProgressRedrawFreq
	p.Format = format
	p.Start()
	return p
}

func downloadProgressLayout(termWidth int) (int, string) {
	if termWidth <= 0 {
		termWidth = 80
	}
	format := "Downloading [{@bar}] <info>{@percent:4s}%</> {@curSize}/{@maxSize}"
	if termWidth >= 120 {
		return 40, format + " ({@elapsed}/{@remaining})"
	}
	if termWidth >= 100 {
		return 32, format + " ({@elapsed}/{@remaining})"
	}
	if termWidth >= 80 {
		return 24, format
	}
	if termWidth >= 64 {
		return 16, format
	}
	return 10, format
}

func (r *InstallRunner) resolveCandidate(target string, candidates []string, opts Options) (string, error) {
	if selected := uniqueCandidateForName(candidates, opts.Name); selected != "" {
		return selected, nil
	}

	previousAssets, _, _ := r.loadInstalled()
	if previous := previousAssets[storepkg.NormalizeRepoName(target)]; previous != "" {
		for _, candidate := range candidates {
			if path.Base(candidate) == previous {
				if r.Stderr != nil {
					ccolor.Fprintf(r.Stderr, "<yellow>Warning: using previous selection '%s' as fallback</>\n", previous)
				}
				return candidate, nil
			}
		}
	}

	if r.Prompt == nil {
		return "", fmt.Errorf("%d candidates found for asset chain", len(candidates))
	}

	choices := make([]string, len(candidates))
	for i, candidate := range candidates {
		choices[i] = path.Base(candidate)
	}
	choice, err := r.Prompt("Select package resource", "Filter assets", choices)
	if err != nil {
		return "", err
	}
	if choice < 0 || choice >= len(candidates) {
		return "", fmt.Errorf("selection %d is out of bounds", choice)
	}
	return candidates[choice], nil
}

func uniqueCandidateForName(candidates []string, name string) string {
	hint := normalizedAssetNameHint(name)
	if hint == "" {
		return ""
	}

	match := ""
	for _, candidate := range candidates {
		if !assetBaseMatchesName(path.Base(candidate), hint) {
			continue
		}
		if match != "" {
			return ""
		}
		match = candidate
	}
	return match
}

func normalizedAssetNameHint(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	for _, suffix := range []string{".exe", ".appimage"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

func assetBaseMatchesName(base, hint string) bool {
	base = strings.ToLower(base)
	if base == hint {
		return true
	}
	if len(base) <= len(hint) || !strings.HasPrefix(base, hint) {
		return false
	}
	switch base[len(hint)] {
	case '-', '_', '.':
		return true
	default:
		return false
	}
}

func (r *InstallRunner) resolveExtractedFile(candidates []ExtractedFile, opts Options) (ExtractedFile, bool, error) {
	goos, goarch := selectionPlatform(opts)
	if selected, ok := autoSelectExtractedFile(candidates, goos, goarch); ok {
		if r.Stderr != nil {
			ccolor.Fprintf(r.Stderr, "🪄 <yellow>Auto-selected extracted file '%s' for %s/%s</>\n", selected.ArchiveName, goos, goarch)
		}
		return selected, false, nil
	}

	if r.Prompt == nil {
		return ExtractedFile{}, false, fmt.Errorf("%d candidates for target found", len(candidates))
	}
	choices := make([]string, len(candidates)+1)
	for i := range candidates {
		choices[i] = candidates[i].String()
	}
	choices[len(candidates)] = "all"
	choice, err := r.Prompt("Select extracted file", "Filter files", choices)
	if err != nil {
		return ExtractedFile{}, false, err
	}
	if choice == len(candidates) {
		return ExtractedFile{}, true, nil
	}
	if choice < 0 || choice >= len(candidates) {
		return ExtractedFile{}, false, fmt.Errorf("selection %d is out of bounds", choice)
	}
	return candidates[choice], false, nil
}

func selectionPlatform(opts Options) (string, string) {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	if opts.System == "" {
		return goos, goarch
	}
	parts := strings.SplitN(opts.System, "/", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1]
	}
	return goos, goarch
}

func autoSelectExtractedFile(candidates []ExtractedFile, goos, goarch string) (ExtractedFile, bool) {
	if len(candidates) == 0 {
		return ExtractedFile{}, false
	}
	if strings.EqualFold(goos, "windows") {
		if selected, ok := autoSelectOnlyWindowsExecutable(candidates); ok {
			return selected, true
		}
	}
	patterns := archSelectionPatterns(goarch)
	if len(patterns) == 0 {
		return ExtractedFile{}, false
	}

	matches := make([]ExtractedFile, 0, len(candidates))
	for _, candidate := range candidates {
		name := util.NormalizeSlashesLower(candidate.ArchiveName)
		for _, pattern := range patterns {
			if pattern.MatchString(name) {
				matches = append(matches, candidate)
				break
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return ExtractedFile{}, false
}

func autoSelectOnlyWindowsExecutable(candidates []ExtractedFile) (ExtractedFile, bool) {
	var selected ExtractedFile
	count := 0
	for _, candidate := range candidates {
		if strings.EqualFold(filepath.Ext(candidate.ArchiveName), ".exe") {
			selected = candidate
			count++
		}
	}
	return selected, count == 1
}

func archSelectionPatterns(goarch string) []*regexp.Regexp {
	switch strings.ToLower(goarch) {
	case "amd64":
		return compileArchPatterns(`(^|/)(x64|amd64|x86_64)(/|$)`)
	case "386":
		return compileArchPatterns(`(^|/)(x86|i386|386)(/|$)`)
	case "arm64":
		return compileArchPatterns(`(^|/)(arm64|aarch64)(/|$)`)
	case "arm":
		return compileArchPatterns(`(^|/)(arm32|armv6|armv7|arm)(/|$)`)
	default:
		return nil
	}
}

func compileArchPatterns(exprs ...string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(exprs))
	for _, expr := range exprs {
		patterns = append(patterns, regexp.MustCompile(expr))
	}
	return patterns
}

func (r *InstallRunner) loadInstalled() (map[string]string, map[string]string, error) {
	if r.InstalledLoad == nil {
		return map[string]string{}, map[string]string{}, nil
	}
	return r.InstalledLoad()
}

func checksumAsset(asset string, assets []string) string {
	for _, candidate := range assets {
		if candidate == asset+".sha256sum" || candidate == asset+".sha256" {
			return candidate
		}
	}
	return ""
}

func outputPath(file ExtractedFile, output string, all bool, preferredName string, outputExplicit bool, renameFiles ...map[string]string) (string, error) {
	mode := file.Mode()
	renamed := renamedOutputName(file, firstRenameMap(renameFiles))
	out := resolvedOutputName(firstNonEmpty(renamed, file.Name), mode, preferredName)
	if all && output != "-" && file.Name != "" {
		if renamed != "" {
			return safeArchiveOutputPath(output, renamed)
		}
		var err error
		out, err = safeArchiveOutputPath(output, file.Name)
		if err != nil {
			return "", err
		}
	}
	if output == "-" {
		return "-", nil
	}
	if all && output != "" && file.Name != "" {
		return out, nil
	}
	if output != "" && all {
		os.MkdirAll(output, 0o755)
		return filepath.Join(output, out), nil
	}
	if output != "" && util.IsDirectory(output) {
		return filepath.Join(output, out), nil
	}
	if output != "" {
		if outputExplicit {
			out = output
		} else {
			if err := os.MkdirAll(output, 0o755); err != nil {
				return "", err
			}
			return filepath.Join(output, out), nil
		}
	}
	if os.Getenv("EGET_BIN") != "" && !strings.ContainsRune(out, os.PathSeparator) && mode&0o111 != 0 && !file.Dir {
		return filepath.Join(os.Getenv("EGET_BIN"), out), nil
	}
	return out, nil
}

func firstRenameMap(items []map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	return items[0]
}

func renamedOutputName(file ExtractedFile, renameFiles map[string]string) string {
	if len(renameFiles) == 0 {
		return ""
	}
	for _, key := range []string{file.ArchiveName, file.Name, filepath.Base(file.ArchiveName), filepath.Base(file.Name)} {
		if key == "" {
			continue
		}
		if renamed := renameFiles[key]; renamed != "" {
			return renamed
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func resolvedOutputName(name string, mode os.FileMode, preferredName string) string {
	base := filepath.Base(name)
	if preferredName != "" {
		return applyPreferredName(base, preferredName)
	}
	if !isExec(base, mode) {
		return base
	}
	return heuristicExecutableName(base)
}

func applyPreferredName(originalName, preferredName string) string {
	ext := executableSuffix(originalName)
	if preferredName == "" {
		return originalName
	}
	if filepath.Ext(preferredName) != "" || ext == "" {
		return preferredName
	}
	return preferredName + ext
}

func heuristicExecutableName(name string) string {
	ext := executableSuffix(name)
	base := strings.TrimSuffix(name, ext)
	patterns := []string{
		`(?i)[-_.](windows|darwin|linux|freebsd|openbsd|netbsd|android|illumos|solaris|plan9|macos|osx)[-_.](amd64|x86_64|x64|386|x86|i386|arm64|aarch64|armv?6|armv?7|arm|riscv64)$`,
		`(?i)[-_.](amd64|x86_64|x64|386|x86|i386|arm64|aarch64|armv?6|armv?7|arm|riscv64)[-_.](windows|darwin|linux|freebsd|openbsd|netbsd|android|illumos|solaris|plan9|macos|osx)$`,
		`(?i)[-_.](windows|darwin|linux|freebsd|openbsd|netbsd|android|illumos|solaris|plan9|macos|osx)$`,
	}
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if trimmed := re.ReplaceAllString(base, ""); trimmed != "" && trimmed != base {
			return trimmed + ext
		}
	}
	return name
}

func executableSuffix(name string) string {
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".exe"):
		return ".exe"
	case strings.HasSuffix(strings.ToLower(name), ".appimage"):
		return ".appimage"
	default:
		return ""
	}
}
