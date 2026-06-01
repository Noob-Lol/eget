package app

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	storepkg "github.com/inherelab/eget/internal/installed"
	forge "github.com/inherelab/eget/internal/source/forge"
	sourcesf "github.com/inherelab/eget/internal/source/sourceforge"
	"github.com/inherelab/eget/internal/util"
)

const (
	maxChunkConcurrency = 32
	maxBatchConcurrency = 16
)

type RunResult = install.RunResult

type Runner interface {
	Run(target string, opts install.Options) (RunResult, error)
}

type InstalledStore interface {
	Record(target string, entry storepkg.Entry) error
}

type PackageAdder interface {
	AddPackage(repo, name string, opts install.Options) error
}

type InstallExtras struct {
	AddToConfig bool
	PackageName string
	PackageOpts install.Options
}

type InstallAllResult struct {
	Name   string
	Target string
	Result RunResult
}

type ReleaseInfoFunc func(repo, url string) (string, time.Time, error)

type Service struct {
	Runner       Runner
	Store        InstalledStore
	Config       PackageAdder
	Now          func() time.Time
	ReleaseInfo  ReleaseInfoFunc
	RepoMetadata func(repo string) (RepoMetadata, error)
	LoadConfig   func() (*cfgpkg.File, error)
}

func (s Service) InstallTarget(target string, opts install.Options, extras ...InstallExtras) (RunResult, error) {
	if err := validateRawConcurrencyOptions(opts); err != nil {
		return RunResult{}, err
	}
	opts, extras = inferAddPackageName(target, opts, extras)
	runTarget, recordTarget, opts, err := s.resolveInstallRequest(target, opts, false)
	if err != nil {
		return RunResult{}, err
	}
	opts = applyDefaultInstallTarget(opts)
	if err := validateConcurrencyOptions(opts); err != nil {
		return RunResult{}, err
	}
	opts = normalizeExtractionOptions(opts)
	result, err := s.installResolvedTarget(runTarget, recordTarget, opts)
	if err != nil {
		return RunResult{}, err
	}

	if len(extras) > 0 && extras[0].AddToConfig {
		if s.Config == nil {
			return RunResult{}, fmt.Errorf("config service is required")
		}
		repo := runTarget
		if normalized, err := install.NormalizeRepoTarget(runTarget); err == nil {
			repo = normalized
		} else if !isManagedConfigTarget(runTarget) {
			return RunResult{}, err
		}
		addOpts := extras[0].PackageOpts
		if result.IsGUI {
			addOpts.IsGUI = true
		}
		if err := s.Config.AddPackage(repo, extras[0].PackageName, addOpts); err != nil {
			return RunResult{}, err
		}
	}

	return result, nil
}

func inferAddPackageName(target string, opts install.Options, extras []InstallExtras) (install.Options, []InstallExtras) {
	if len(extras) == 0 || !extras[0].AddToConfig || opts.Name != "" || extras[0].PackageName != "" {
		return opts, extras
	}
	name, err := ResolvePackageName(target, "")
	if err != nil || name == "" {
		return opts, extras
	}
	opts.Name = name
	extras[0].PackageName = name
	extras[0].PackageOpts.Name = name
	return opts, extras
}

func applyDefaultInstallTarget(opts install.Options) install.Options {
	if opts.Output == "" {
		opts.Output = defaultInstallTarget()
		opts.OutputExplicit = false
	}
	return opts
}

func defaultInstallTarget() string {
	target, err := util.Expand("~/.local/bin")
	if err != nil || target == "" {
		return "~/.local/bin"
	}
	return target
}

func (s Service) InstallAllPackages(cli install.Options) ([]InstallAllResult, error) {
	if err := validateRawConcurrencyOptions(cli); err != nil {
		return nil, err
	}
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if len(cfg.Packages) == 0 {
		return nil, fmt.Errorf("no managed packages configured")
	}

	names := make([]string, 0, len(cfg.Packages))
	for name := range cfg.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	rawBatch := batchConcurrencyFromConfig(cfg, cli)
	if err := validateConcurrencyOptions(install.Options{BatchConcurrency: rawBatch}); err != nil {
		return nil, err
	}
	batch := effectiveBatchConcurrency(rawBatch, len(names))
	if batch > 1 {
		return s.installAllPackagesConcurrent(cfg, names, cli, batch)
	}

	results := make([]InstallAllResult, 0, len(names))
	for _, name := range names {
		pkg := cfg.Packages[name]
		repo := util.DerefString(pkg.Repo)
		if repo == "" {
			return nil, fmt.Errorf("package %q has no repo", name)
		}
		runTarget, recordTarget, opts, err := s.resolveInstallRequestWithConfig(cfg, name, cli, false)
		if err != nil {
			return nil, err
		}
		if err := validateConcurrencyOptions(opts); err != nil {
			return nil, err
		}
		opts = applyDefaultInstallTarget(opts)
		opts = normalizeExtractionOptions(opts)
		result, err := s.installResolvedTarget(runTarget, recordTarget, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, InstallAllResult{
			Name:   name,
			Target: runTarget,
			Result: result,
		})
	}
	return results, nil
}

func (s Service) installAllPackagesConcurrent(cfg *cfgpkg.File, names []string, cli install.Options, batch int) ([]InstallAllResult, error) {
	type job struct {
		index int
		name  string
	}
	results := make([]InstallAllResult, len(names))
	jobs := make(chan job)
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < batch; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				select {
				case <-ctx.Done():
					continue
				default:
				}
				runTarget, recordTarget, opts, err := s.resolveInstallRequestWithConfig(cfg, item.name, cli, false)
				if err != nil {
					sendFirstError(errCh, err, cancel)
					continue
				}
				if err := validateConcurrencyOptions(opts); err != nil {
					sendFirstError(errCh, err, cancel)
					continue
				}
				opts = applyDefaultInstallTarget(opts)
				opts = normalizeExtractionOptions(opts)
				result, err := s.installResolvedTarget(runTarget, recordTarget, opts)
				if err != nil {
					sendFirstError(errCh, err, cancel)
					continue
				}
				results[item.index] = InstallAllResult{Name: item.name, Target: runTarget, Result: result}
			}
		}()
	}

sendLoop:
	for index, name := range names {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- job{index: index, name: name}:
		}
	}
	close(jobs)
	wg.Wait()

	select {
	case err := <-errCh:
		return nil, err
	default:
	}
	return results, nil
}

func isManagedConfigTarget(target string) bool {
	switch install.DetectTargetKind(target) {
	case install.TargetRepo, install.TargetGitHubURL, install.TargetSourceForge, install.TargetForge, install.TargetTemplate:
		return true
	default:
		return false
	}
}

func sourceVersion(tag string, sourceBacked bool) string {
	if sourceBacked {
		return tag
	}
	return ""
}

func (s Service) resolveInstallRequest(target string, cli install.Options, preferCacheDir bool) (string, string, install.Options, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return "", "", install.Options{}, err
	}
	return s.resolveInstallRequestWithConfig(cfg, target, cli, preferCacheDir)
}

func (s Service) resolveInstallRequestWithConfig(cfg *cfgpkg.File, target string, cli install.Options, preferCacheDir bool) (string, string, install.Options, error) {
	if pkg, ok := cfg.Packages[target]; ok {
		repo := util.DerefString(pkg.Repo)
		if repo == "" {
			return "", "", install.Options{}, fmt.Errorf("package %q has no repo", target)
		}
		opts, err := s.resolveInstallOptionsWithConfig(cfg, repo, pkg, cli, preferCacheDir)
		if err != nil {
			return "", "", install.Options{}, err
		}
		return repo, target, opts, nil
	}

	pkg := packageSectionForRepoTarget(cfg, target)
	opts, err := s.resolveInstallOptionsWithConfig(cfg, target, pkg, cli, preferCacheDir)
	if err != nil {
		return "", "", install.Options{}, err
	}
	return target, installRecordTarget(target, opts), opts, nil
}

func installRecordTarget(target string, opts install.Options) string {
	if opts.Name != "" {
		return opts.Name
	}
	if normalized, err := install.NormalizeRepoTarget(target); err == nil {
		return repoName(normalized)
	}
	return target
}

func packageSectionForRepoTarget(cfg *cfgpkg.File, target string) cfgpkg.Section {
	targetRepo, err := install.NormalizeRepoTarget(target)
	if err != nil {
		return cfgpkg.Section{}
	}
	for _, pkg := range cfg.Packages {
		repo := util.DerefString(pkg.Repo)
		if repo == "" {
			continue
		}
		normalized, err := install.NormalizeRepoTarget(repo)
		if err != nil {
			continue
		}
		if normalized == targetRepo {
			return pkg
		}
	}
	return cfgpkg.Section{}
}

func (s Service) installResolvedTarget(runTarget, recordTarget string, opts install.Options) (RunResult, error) {
	result, err := s.Runner.Run(runTarget, opts)
	if err != nil {
		return RunResult{}, err
	}

	installMode := result.InstallMode
	if installMode == "" && opts.IsGUI && len(result.ExtractedFiles) > 0 {
		installMode = install.InstallModePortable
	}
	shouldRecord := len(result.ExtractedFiles) > 0 || installMode == install.InstallModeInstaller || installMode == install.InstallModeRunAsset
	if s.Store == nil || !shouldRecord {
		return result, nil
	}

	repo := storepkg.NormalizeRepoName(runTarget)
	tag, releaseDate := tagFromReleaseURL(result.URL), time.Time{}
	isSourceForge := sourcesf.IsTarget(repo)
	isForge := forge.IsTarget(repo)
	isTemplate := install.DetectTargetKind(runTarget) == install.TargetTemplate
	if tag == "" && isTemplate {
		tag = result.Version
	}
	if tag == "" && isSourceForge {
		tag = sourcesf.VersionFromText(result.URL)
	}
	if tag == "" && isForge && opts.Tag != "" {
		tag = opts.Tag
	}
	if tag == "" && s.ReleaseInfo != nil && shouldFetchReleaseInfo(repo) {
		if gotTag, gotDate, err := s.ReleaseInfo(repo, result.URL); err == nil {
			if tag == "" {
				tag = gotTag
			}
			releaseDate = gotDate
		}
	}
	meta := RepoMetadata{}
	if shouldFetchRepoMetadata(repo) {
		meta = s.repoMetadata(repo)
	}
	if desc := s.configDescForRepo(repo); desc != "" {
		meta.Desc = desc
	}
	if meta.Homepage == "" {
		meta.Homepage = inferRepoWebURL(repo)
	}
	if meta.RepoURL == "" {
		meta.RepoURL = inferRepoWebURL(repo)
	}

	entry := storepkg.Entry{
		Repo:           repo,
		Target:         runTarget,
		InstalledAt:    s.now(),
		URL:            result.URL,
		Asset:          chooseAsset(result),
		Desc:           meta.Desc,
		Homepage:       meta.Homepage,
		RepoURL:        meta.RepoURL,
		Tool:           result.Tool,
		ExtractedFiles: append([]string(nil), result.ExtractedFiles...),
		Options:        extractOptionsMap(opts, result.IsGUI || opts.IsGUI),
		Tag:            tag,
		Version:        sourceVersion(tag, isSourceForge || isForge || isTemplate),
		ReleaseDate:    releaseDate,
		IsGUI:          result.IsGUI || opts.IsGUI,
		InstallMode:    installMode,
	}
	if err := s.Store.Record(recordTarget, entry); err != nil {
		return RunResult{}, err
	}
	return result, nil
}

func shouldFetchReleaseInfo(repo string) bool {
	if install.DetectTargetKind(repo) == install.TargetTemplate {
		return false
	}
	if sourcesf.IsTarget(repo) {
		return false
	}
	if forge.IsTarget(repo) {
		return true
	}
	return isGitHubRepoTarget(repo)
}

func shouldFetchRepoMetadata(repo string) bool {
	return isGitHubRepoTarget(repo)
}

func isGitHubRepoTarget(repo string) bool {
	return install.DetectTargetKind(repo) == install.TargetRepo || install.DetectTargetKind(repo) == install.TargetGitHubURL
}

func (s Service) configDescForRepo(repo string) string {
	cfg, err := s.loadConfig()
	if err != nil || cfg == nil {
		return ""
	}
	pkg := packageSectionForRepoTarget(cfg, repo)
	return util.DerefString(pkg.Desc)
}

func (s Service) repoMetadata(repo string) RepoMetadata {
	if s.RepoMetadata == nil {
		return RepoMetadata{}
	}
	meta, err := s.RepoMetadata(repo)
	if err != nil {
		return RepoMetadata{}
	}
	return meta
}

func (s Service) DownloadTarget(target string, opts install.Options) (RunResult, error) {
	if err := validateRawConcurrencyOptions(opts); err != nil {
		return RunResult{}, err
	}
	runTarget, _, opts, err := s.resolveInstallRequest(target, opts, true)
	if err != nil {
		return RunResult{}, err
	}
	if err := validateConcurrencyOptions(opts); err != nil {
		return RunResult{}, err
	}
	opts = normalizeExtractionOptions(opts)
	opts.DownloadOnly = opts.ExtractFile == "" && !opts.All
	return s.Runner.Run(runTarget, opts)
}

func normalizeExtractionOptions(opts install.Options) install.Options {
	if hasMultipleExtractPatterns(opts.ExtractFile) {
		opts.All = true
	}
	if opts.ExtractFile != "" || opts.All {
		opts.DownloadOnly = false
	}
	return opts
}

func hasMultipleExtractPatterns(value string) bool {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(value, ",") {
			return true
		}
		if strings.ContainsAny(part, "*?[{") {
			return true
		}
	}
	return false
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func chooseAsset(result RunResult) string {
	if result.Asset != "" {
		return result.Asset
	}
	return path.Base(result.URL)
}

func tagFromReleaseURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] == "releases" && parts[i+1] == "download" {
			return releaseTagFromPathParts(parts[i+2 : len(parts)-1])
		}
		if parts[i] == "releases" {
			for j := i + 2; j+1 < len(parts); j++ {
				if parts[j] == "downloads" {
					return releaseTagFromPathParts(parts[i+1 : j])
				}
			}
		}
	}
	return ""
}

func releaseTagFromPathParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	raw := strings.Join(parts, "/")
	tag, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return tag
}

func (s Service) loadConfig() (*cfgpkg.File, error) {
	if s.LoadConfig != nil {
		return s.LoadConfig()
	}
	return cfgpkg.Load()
}

func (s Service) resolveInstallOptions(target string, cli install.Options, preferCacheDir bool) (install.Options, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return install.Options{}, err
	}
	return s.resolveInstallOptionsWithConfig(cfg, target, cfgpkg.Section{}, cli, preferCacheDir)
}

func (s Service) resolveInstallOptionsWithConfig(cfg *cfgpkg.File, target string, pkg cfgpkg.Section, cli install.Options, preferCacheDir bool) (install.Options, error) {
	repoKey := ""
	if repo, err := install.NormalizeRepoTarget(target); err == nil {
		repoKey = repo
	}

	merged := cfgpkg.MergeInstallOptions(cfg.Global, cfg.Repos[repoKey], pkg, cfgpkg.CLIOverrides{
		ExtractAll:       boolOpt(cli.All),
		AssetFilters:     stringsOpt(cli.Asset),
		CacheDir:         stringOpt(cli.CacheDir),
		ProxyURL:         stringOpt(cli.ProxyURL),
		DownloadOnly:     boolOpt(cli.DownloadOnly),
		File:             stringOpt(cli.ExtractFile),
		IsGUI:            boolOpt(cli.IsGUI),
		InstallMode:      stringOpt(cli.InstallMode),
		Name:             stringOpt(cli.Name),
		Quiet:            boolOpt(cli.Quiet),
		RenameFiles:      mapOpt(cli.RenameFiles),
		ShowHash:         boolOpt(cli.Hash),
		StripComponents:  intOpt(cli.StripComponents, cli.StripComponents > 0),
		Source:           boolOpt(cli.Source),
		SourcePath:       stringOpt(cli.SourcePath),
		System:           stringOpt(cli.System),
		Tag:              stringOpt(cli.Tag),
		Target:           stringOpt(cli.Output),
		UpgradeOnly:      boolOpt(cli.UpgradeOnly),
		Verify:           stringOpt(cli.Verify),
		DisableSSL:       boolOpt(cli.DisableSSL),
		ChunkConcurrency: intOpt(cli.ChunkConcurrency, cli.ChunkConcurrencySet),
	})

	targetDir, err := expandPath(merged.Target)
	if err != nil {
		return install.Options{}, err
	}
	cacheDir, err := expandPath(merged.CacheDir)
	if err != nil {
		return install.Options{}, err
	}
	guiTarget, err := expandPath(merged.GuiTarget)
	if err != nil {
		return install.Options{}, err
	}
	sys7zPath, err := expandPath(merged.Sys7zPath)
	if err != nil {
		return install.Options{}, err
	}
	apiCacheDir := ""
	if cacheDir != "" {
		apiCacheDir = filepath.Join(cacheDir, "api-cache")
	}

	output := targetDir
	if preferCacheDir && cli.Output == "" && cacheDir != "" {
		output = cacheDir
	}

	apiCacheEnabled := false
	if cfg.ApiCache.Enable != nil {
		apiCacheEnabled = *cfg.ApiCache.Enable
	}
	apiCacheTime := 0
	if cfg.ApiCache.CacheTime != nil {
		apiCacheTime = *cfg.ApiCache.CacheTime
	}
	ghproxyEnabled := false
	if cfg.Ghproxy.Enable != nil {
		ghproxyEnabled = *cfg.Ghproxy.Enable
	}
	ghproxyHostURL := util.DerefString(cfg.Ghproxy.HostURL)
	ghproxySupportAPI := false
	if cfg.Ghproxy.SupportAPI != nil {
		ghproxySupportAPI = *cfg.Ghproxy.SupportAPI
	}
	batchConcurrency := 0
	if cli.BatchConcurrencySet || cli.BatchConcurrency > 0 {
		batchConcurrency = cli.BatchConcurrency
	} else if cfg.Global.BatchConcurrency != nil {
		batchConcurrency = *cfg.Global.BatchConcurrency
	}

	cacheName := merged.Name
	if cli.CacheName != "" {
		cacheName = cli.CacheName
	}
	cacheVersion := merged.Tag
	if cli.CacheVersion != "" {
		cacheVersion = cli.CacheVersion
	}

	return install.Options{
		Tag:                 merged.Tag,
		Name:                merged.Name,
		Source:              merged.Source,
		SourcePath:          merged.SourcePath,
		Sys7zPath:           sys7zPath,
		Output:              output,
		OutputExplicit:      cli.Output != "",
		GuiTarget:           guiTarget,
		IsGUI:               merged.IsGUI,
		InstallMode:         merged.InstallMode,
		CacheDir:            cacheDir,
		CacheName:           cacheName,
		CacheVersion:        cacheVersion,
		ProxyURL:            merged.ProxyURL,
		UserAgent:           merged.UserAgent,
		APICacheEnabled:     apiCacheEnabled,
		APICacheDir:         apiCacheDir,
		APICacheTime:        apiCacheTime,
		GhproxyEnabled:      ghproxyEnabled,
		GhproxyHostURL:      ghproxyHostURL,
		GhproxySupportAPI:   ghproxySupportAPI,
		GhproxyFallbacks:    append([]string(nil), cfg.Ghproxy.Fallbacks...),
		System:              merged.System,
		ExtractFile:         merged.File,
		All:                 merged.ExtractAll,
		StripComponents:     merged.StripComponents,
		Quiet:               merged.Quiet,
		DownloadOnly:        merged.DownloadOnly,
		FallbackVersions:    cli.FallbackVersions,
		ChunkConcurrency:    merged.ChunkConcurrency,
		BatchConcurrency:    batchConcurrency,
		ChunkConcurrencySet: true,
		BatchConcurrencySet: true,
		UpgradeOnly:         merged.UpgradeOnly,
		Asset:               append([]string(nil), merged.AssetFilters...),
		RenameFiles:         cloneStringMap(merged.RenameFiles),
		Hash:                merged.ShowHash,
		Verify:              merged.Verify,
		URLTemplate: install.URLTemplateOptions{
			URLTemplate:         merged.URLTemplate,
			LatestURL:           merged.LatestURL,
			LatestFormat:        merged.LatestFormat,
			LatestJSONPath:      merged.LatestJSONPath,
			VersionRegex:        merged.VersionRegex,
			OSMap:               cloneStringMap(merged.OSMap),
			ArchMap:             cloneStringMap(merged.ArchMap),
			ExtMap:              cloneStringMap(merged.ExtMap),
			LibcMap:             cloneStringMap(merged.LibcMap),
			ChecksumURLTemplate: merged.ChecksumURLTemplate,
			ChecksumFormat:      merged.ChecksumFormat,
			ChecksumJSONPath:    merged.ChecksumJSONPath,
			ChecksumRegex:       merged.ChecksumRegex,
			InstallAction:       merged.InstallAction,
			InstallArgs:         append([]string(nil), merged.InstallArgs...),
		},
		DisableSSL: merged.DisableSSL,
	}, nil
}

func validateConcurrencyOptions(opts install.Options) error {
	if opts.ChunkConcurrency < 0 || opts.ChunkConcurrency > maxChunkConcurrency {
		return fmt.Errorf("chunk concurrency must be between 0 and %d", maxChunkConcurrency)
	}
	if opts.BatchConcurrency < 0 || opts.BatchConcurrency > maxBatchConcurrency {
		return fmt.Errorf("batch concurrency must be between 0 and %d", maxBatchConcurrency)
	}
	return nil
}

func validateRawConcurrencyOptions(opts install.Options) error {
	if opts.ChunkConcurrency < -1 || opts.BatchConcurrency < -1 {
		return fmt.Errorf("concurrency must be 0 auto, 1 serial/single connection, or greater than 1")
	}
	return nil
}

func batchConcurrencyFromConfig(cfg *cfgpkg.File, cli install.Options) int {
	if cli.BatchConcurrencySet || cli.BatchConcurrency > 0 {
		return cli.BatchConcurrency
	}
	if cfg != nil && cfg.Global.BatchConcurrency != nil {
		return *cfg.Global.BatchConcurrency
	}
	return 0
}

func effectiveBatchConcurrency(value, total int) int {
	if total <= 1 {
		return 1
	}
	if value <= 0 {
		return 1
	}
	if value > total {
		return total
	}
	return value
}

func sendFirstError(errCh chan<- error, err error, cancel func()) {
	select {
	case errCh <- err:
		cancel()
	default:
	}
}

func mapOpt(value map[string]string) *map[string]string {
	if len(value) == 0 {
		return nil
	}
	cloned := cloneStringMap(value)
	return &cloned
}

func cloneStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func expandPath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return util.Expand(value)
}

func extractOptionsMap(opts install.Options, isGUI bool) map[string]interface{} {
	recorded := make(map[string]interface{})
	if opts.Tag != "" {
		recorded["tag"] = opts.Tag
	}
	if opts.System != "" {
		recorded["system"] = opts.System
	}
	if opts.Output != "" {
		recorded["output"] = opts.Output
	}
	if isGUI && opts.GuiTarget != "" {
		recorded["gui_target"] = opts.GuiTarget
	}
	if isGUI {
		recorded["is_gui"] = true
	}
	if opts.InstallMode != "" {
		recorded["install_mode"] = opts.InstallMode
	}
	if opts.ExtractFile != "" {
		recorded["extract_file"] = opts.ExtractFile
	}
	if opts.All {
		recorded["all"] = true
	}
	if opts.StripComponents > 0 {
		recorded["strip_components"] = opts.StripComponents
	}
	if opts.Quiet {
		recorded["quiet"] = true
	}
	if opts.DownloadOnly {
		recorded["download_only"] = true
	}
	if opts.UpgradeOnly {
		recorded["upgrade_only"] = true
	}
	if len(opts.Asset) > 0 {
		recorded["asset"] = append([]string(nil), opts.Asset...)
	}
	if len(opts.RenameFiles) > 0 {
		recorded["rename_files"] = cloneStringMap(opts.RenameFiles)
	}
	if opts.Hash {
		recorded["hash"] = true
	}
	if opts.Verify != "" {
		recorded["verify"] = opts.Verify
	}
	if opts.Source {
		recorded["download_source"] = true
	}
	if opts.DisableSSL {
		recorded["disable_ssl"] = true
	}
	if opts.URLTemplate.URLTemplate != "" {
		recorded["url_template"] = opts.URLTemplate.URLTemplate
	}
	if opts.URLTemplate.LatestURL != "" {
		recorded["latest_url"] = opts.URLTemplate.LatestURL
	}
	if opts.URLTemplate.LatestFormat != "" {
		recorded["latest_format"] = opts.URLTemplate.LatestFormat
	}
	if opts.URLTemplate.LatestJSONPath != "" {
		recorded["latest_json_path"] = opts.URLTemplate.LatestJSONPath
	}
	if opts.URLTemplate.VersionRegex != "" {
		recorded["version_regex"] = opts.URLTemplate.VersionRegex
	}
	if len(opts.URLTemplate.OSMap) > 0 {
		recorded["os_map"] = cloneStringMap(opts.URLTemplate.OSMap)
	}
	if len(opts.URLTemplate.ArchMap) > 0 {
		recorded["arch_map"] = cloneStringMap(opts.URLTemplate.ArchMap)
	}
	if len(opts.URLTemplate.ExtMap) > 0 {
		recorded["ext_map"] = cloneStringMap(opts.URLTemplate.ExtMap)
	}
	if len(opts.URLTemplate.LibcMap) > 0 {
		recorded["libc_map"] = cloneStringMap(opts.URLTemplate.LibcMap)
	}
	if opts.URLTemplate.ChecksumURLTemplate != "" {
		recorded["checksum_url_template"] = opts.URLTemplate.ChecksumURLTemplate
	}
	if opts.URLTemplate.ChecksumFormat != "" {
		recorded["checksum_format"] = opts.URLTemplate.ChecksumFormat
	}
	if opts.URLTemplate.ChecksumJSONPath != "" {
		recorded["checksum_json_path"] = opts.URLTemplate.ChecksumJSONPath
	}
	if opts.URLTemplate.ChecksumRegex != "" {
		recorded["checksum_regex"] = opts.URLTemplate.ChecksumRegex
	}
	if opts.URLTemplate.InstallAction != "" {
		recorded["install_action"] = opts.URLTemplate.InstallAction
	}
	if len(opts.URLTemplate.InstallArgs) > 0 {
		recorded["install_args"] = append([]string(nil), opts.URLTemplate.InstallArgs...)
	}
	return recorded
}
