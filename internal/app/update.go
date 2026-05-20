package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	storepkg "github.com/inherelab/eget/internal/installed"
	sourcesf "github.com/inherelab/eget/internal/source/sourceforge"
	"github.com/inherelab/eget/internal/util"
)

type Installer interface {
	InstallTarget(target string, opts install.Options, extras ...InstallExtras) (RunResult, error)
}

type UpdateService struct {
	Install       Installer
	LoadConfig    func() (*cfgpkg.File, error)
	LoadInstalled func() (*storepkg.Config, error)
	LatestInfo    func(repo, sourcePath string) (LatestInfo, error)
	OnCheckDone   func(checked, total int)
}

type UpdateResult struct {
	Name   string
	Target string
	Result RunResult
}

func (s UpdateService) UpdatePackage(nameOrRepo string, cli install.Options) (RunResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return RunResult{}, err
	}
	installed, err := s.loadInstalled()
	if err != nil {
		return RunResult{}, err
	}

	item, entry, managed, ok := findUpdateTarget(cfg, installed, nameOrRepo)
	if !ok {
		return RunResult{}, fmt.Errorf("update target %q is not configured or installed; use install first", nameOrRepo)
	}
	if !item.Installed {
		return RunResult{}, fmt.Errorf("update target %q is not installed; use install first", nameOrRepo)
	}
	if s.LatestInfo == nil {
		return RunResult{}, fmt.Errorf("latest info checker is required")
	}
	enrichListItemFromInstalledEntry(&item, entry)
	check := checkOutdatedItem(item, s.LatestInfo)
	if check.failure != nil {
		return RunResult{}, check.failure.Error
	}
	if check.outdated == nil {
		return RunResult{}, nil
	}

	target := item.Name
	opts := cli
	if !managed {
		target = installedUpdateTarget(item, entry)
		opts = applyUpdateCLIOverrides(optionsFromInstalledEntry(entry), cli)
	}
	return s.Install.InstallTarget(target, opts)
}

func (s UpdateService) UpdateAllPackages(cli install.Options) ([]UpdateResult, error) {
	candidates, _, _, err := s.ListUpdateCandidates()
	if err != nil {
		return nil, err
	}

	return s.UpdateCandidates(candidates, cli)
}

func findUpdateTarget(cfg *cfgpkg.File, installed *storepkg.Config, target string) (ListItem, storepkg.Entry, bool, bool) {
	if cfg == nil {
		cfg = cfgpkg.NewFile()
	}
	if installed == nil {
		installed = &storepkg.Config{}
	}
	items, err := (ListService{
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
		LoadInstalled: func() (*storepkg.Config, error) {
			return installed, nil
		},
	}).ListPackages()
	if err != nil {
		return ListItem{}, storepkg.Entry{}, false, false
	}

	normalizedTarget := storepkg.NormalizeRepoName(target)
	for _, item := range items {
		entry := installedEntryForItem(installed, item)
		if _, managed := cfg.Packages[item.Name]; managed && target == item.Name {
			return item, entry, true, true
		}
		if item.Repo == target || item.Repo == normalizedTarget || entry.Target == target || storepkg.NormalizeRepoName(entry.Target) == normalizedTarget {
			_, managed := cfg.Packages[item.Name]
			return item, entry, managed, true
		}
	}
	return ListItem{}, storepkg.Entry{}, false, false
}

func installedEntryForItem(installed *storepkg.Config, item ListItem) storepkg.Entry {
	if installed == nil || installed.Installed == nil {
		return storepkg.Entry{}
	}
	if entry, ok := installed.Installed[item.Repo]; ok {
		return entry
	}
	normalized := storepkg.NormalizeRepoName(item.Repo)
	if entry, ok := installed.Installed[normalized]; ok {
		return entry
	}
	return storepkg.Entry{}
}

func enrichListItemFromInstalledEntry(item *ListItem, entry storepkg.Entry) {
	if item == nil {
		return
	}
	if item.SourcePath == "" {
		if sourcePath, ok := stringOption(entry.Options, "source_path"); ok {
			item.SourcePath = sourcePath
		} else if sfTarget, err := sourcesf.ParseTarget(entry.Target); err == nil {
			item.SourcePath = sfTarget.Path
		}
	}
}

func installedUpdateTarget(item ListItem, entry storepkg.Entry) string {
	if entry.Target != "" {
		return entry.Target
	}
	if item.Repo != "" {
		return item.Repo
	}
	return entry.Repo
}

func optionsFromInstalledEntry(entry storepkg.Entry) install.Options {
	opts := install.Options{}
	if entry.Options == nil {
		return opts
	}
	opts.Tag, _ = stringOption(entry.Options, "tag")
	opts.System, _ = stringOption(entry.Options, "system")
	opts.SourcePath, _ = stringOption(entry.Options, "source_path")
	opts.Output, _ = stringOption(entry.Options, "output", "target")
	opts.OutputExplicit = opts.Output != ""
	opts.GuiTarget, _ = stringOption(entry.Options, "gui_target")
	opts.ExtractFile, _ = stringOption(entry.Options, "extract_file", "file")
	opts.Verify, _ = stringOption(entry.Options, "verify")
	opts.All, _ = boolOption(entry.Options, "all", "extract_all")
	opts.IsGUI, _ = boolOption(entry.Options, "is_gui")
	opts.Quiet, _ = boolOption(entry.Options, "quiet")
	opts.DownloadOnly, _ = boolOption(entry.Options, "download_only")
	opts.UpgradeOnly, _ = boolOption(entry.Options, "upgrade_only")
	opts.Source, _ = boolOption(entry.Options, "download_source", "source")
	opts.DisableSSL, _ = boolOption(entry.Options, "disable_ssl")
	opts.Asset = stringSliceOption(entry.Options, "asset", "asset_filters")
	opts.RenameFiles = stringMapOption(entry.Options, "rename_files")
	return opts
}

func applyUpdateCLIOverrides(base, cli install.Options) install.Options {
	if cli.Tag != "" {
		base.Tag = cli.Tag
	}
	if cli.Source {
		base.Source = true
	}
	if cli.SourcePath != "" {
		base.SourcePath = cli.SourcePath
	}
	if cli.Output != "" {
		base.Output = cli.Output
		base.OutputExplicit = cli.OutputExplicit
	}
	if cli.System != "" {
		base.System = cli.System
	}
	if cli.ExtractFile != "" {
		base.ExtractFile = cli.ExtractFile
	}
	if cli.All {
		base.All = true
	}
	if cli.Quiet {
		base.Quiet = true
	}
	if cli.ChunkConcurrencySet {
		base.ChunkConcurrency = cli.ChunkConcurrency
		base.ChunkConcurrencySet = true
	}
	if cli.BatchConcurrencySet {
		base.BatchConcurrency = cli.BatchConcurrency
		base.BatchConcurrencySet = true
	}
	if len(cli.Asset) > 0 {
		base.Asset = append([]string(nil), cli.Asset...)
	}
	if len(cli.RenameFiles) > 0 {
		base.RenameFiles = cloneStringMap(cli.RenameFiles)
	}
	return base
}

func stringOption(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text, ok := value.(string); ok && text != "" {
				return text, true
			}
		}
	}
	return "", false
}

func boolOption(values map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if enabled, ok := value.(bool); ok {
				return enabled, true
			}
		}
	}
	return false, false
}

func stringSliceOption(values map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []string:
			return append([]string(nil), typed...)
		case []any:
			items := make([]string, 0, len(typed))
			for _, item := range typed {
				if text, ok := item.(string); ok {
					items = append(items, text)
				}
			}
			return items
		case string:
			if typed != "" {
				return strings.Split(typed, ",")
			}
		}
	}
	return nil
}

func stringMapOption(values map[string]any, keys ...string) map[string]string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case map[string]string:
			return cloneStringMap(typed)
		case map[string]any:
			items := make(map[string]string, len(typed))
			for from, to := range typed {
				if text, ok := to.(string); ok {
					items[from] = text
				}
			}
			if len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

func (s UpdateService) ListUpdateCandidates() ([]OutdatedItem, []OutdatedCheckFailure, int, error) {
	if s.LatestInfo == nil {
		return nil, nil, 0, fmt.Errorf("latest info checker is required")
	}

	cfg, err := s.loadConfig()
	if err != nil {
		return nil, nil, 0, err
	}

	managedNames := make(map[string]bool, len(cfg.Packages))
	managedRepos := make(map[string]bool, len(cfg.Packages))
	for name, pkg := range cfg.Packages {
		managedNames[name] = true
		if repo := util.DerefString(pkg.Repo); repo != "" {
			managedRepos[repo] = true
		}
	}

	listService := ListService{
		LoadConfig: func() (*cfgpkg.File, error) {
			return cfg, nil
		},
		LoadInstalled: s.loadInstalled,
		LatestInfo:    s.LatestInfo,
	}
	items, err := listService.ListPackages()
	if err != nil {
		return nil, nil, 0, err
	}

	outdated, failures, checked := checkOutdatedItems(items, s.LatestInfo, func(item ListItem) bool {
		return managedNames[item.Name] || managedRepos[item.Repo]
	}, batchConcurrencyFromConfig(cfg, install.Options{}), s.OnCheckDone)
	return outdated, failures, checked, nil
}

func (s UpdateService) UpdateCandidates(candidates []OutdatedItem, cli install.Options) ([]UpdateResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	cli = applyConfigNetworkOptions(cfg, cli)
	if err := validateRawConcurrencyOptions(cli); err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	rawBatch := cli.BatchConcurrency
	if !cli.BatchConcurrencySet && rawBatch <= 0 {
		rawBatch = 0
	}
	if err := validateConcurrencyOptions(install.Options{BatchConcurrency: rawBatch}); err != nil {
		return nil, err
	}
	batch := effectiveBatchConcurrency(rawBatch, len(candidates))
	if batch > 1 {
		return s.updateCandidatesConcurrent(candidates, cli, batch)
	}

	results := make([]UpdateResult, 0, len(candidates))
	for _, item := range candidates {
		result, err := s.UpdatePackage(item.Name, cli)
		if err != nil {
			return nil, err
		}
		results = append(results, UpdateResult{
			Name:   item.Name,
			Target: item.Repo,
			Result: result,
		})
	}
	return results, nil
}

func (s UpdateService) updateCandidatesConcurrent(candidates []OutdatedItem, cli install.Options, batch int) ([]UpdateResult, error) {
	type job struct {
		index int
		item  OutdatedItem
	}
	results := make([]UpdateResult, len(candidates))
	jobs := make(chan job)
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < batch; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range jobs {
				select {
				case <-ctx.Done():
					continue
				default:
				}
				result, err := s.UpdatePackage(work.item.Name, cli)
				if err != nil {
					sendFirstError(errCh, err, cancel)
					continue
				}
				results[work.index] = UpdateResult{Name: work.item.Name, Target: work.item.Repo, Result: result}
			}
		}()
	}

sendLoop:
	for index, item := range candidates {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- job{index: index, item: item}:
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

func (s UpdateService) loadConfig() (*cfgpkg.File, error) {
	if s.LoadConfig != nil {
		return s.LoadConfig()
	}
	return cfgpkg.Load()
}

func (s UpdateService) loadInstalled() (*storepkg.Config, error) {
	if s.LoadInstalled != nil {
		return s.LoadInstalled()
	}
	store, err := storepkg.DefaultStore()
	if err != nil {
		return nil, err
	}
	return store.Load()
}

func applyConfigNetworkOptions(cfg *cfgpkg.File, opts install.Options) install.Options {
	if cfg == nil {
		return opts
	}
	if opts.CacheDir == "" {
		opts.CacheDir, _ = expandPath(util.DerefString(cfg.Global.CacheDir))
	}
	if opts.ProxyURL == "" {
		opts.ProxyURL = util.DerefString(cfg.Global.ProxyURL)
	}
	if cfg.ApiCache.Enable != nil {
		opts.APICacheEnabled = *cfg.ApiCache.Enable
	}
	if cfg.ApiCache.CacheTime != nil {
		opts.APICacheTime = *cfg.ApiCache.CacheTime
	}
	if opts.APICacheDir == "" && opts.CacheDir != "" {
		opts.APICacheDir = filepath.Join(opts.CacheDir, "api-cache")
	}
	if cfg.Ghproxy.Enable != nil {
		opts.GhproxyEnabled = *cfg.Ghproxy.Enable
	}
	if opts.GhproxyHostURL == "" {
		opts.GhproxyHostURL = util.DerefString(cfg.Ghproxy.HostURL)
	}
	if cfg.Ghproxy.SupportAPI != nil {
		opts.GhproxySupportAPI = *cfg.Ghproxy.SupportAPI
	}
	if len(opts.GhproxyFallbacks) == 0 && len(cfg.Ghproxy.Fallbacks) > 0 {
		opts.GhproxyFallbacks = append([]string(nil), cfg.Ghproxy.Fallbacks...)
	}
	return opts
}

func boolOpt(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}

func stringOpt(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringsOpt(value []string) *[]string {
	if len(value) == 0 {
		return nil
	}
	copied := append([]string(nil), value...)
	return &copied
}

func intOpt(value int, explicit bool) *int {
	if value < 0 {
		return nil
	}
	if !explicit && value == 0 {
		return nil
	}
	return &value
}
