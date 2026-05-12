package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	storepkg "github.com/inherelab/eget/internal/installed"
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

	if pkg, ok := cfg.Packages[nameOrRepo]; ok {
		if util.DerefString(pkg.Repo) == "" {
			return RunResult{}, fmt.Errorf("package %q has no repo", nameOrRepo)
		}
		return s.Install.InstallTarget(nameOrRepo, cli)
	}

	if isDirectUpdateTarget(nameOrRepo) {
		return s.Install.InstallTarget(nameOrRepo, cli)
	}

	return RunResult{}, fmt.Errorf("unknown package %q", nameOrRepo)
}

func isDirectUpdateTarget(target string) bool {
	switch install.DetectTargetKind(target) {
	case install.TargetRepo,
		install.TargetGitHubURL,
		install.TargetDirectURL,
		install.TargetLocalFile,
		install.TargetSourceForge,
		install.TargetForge:
		return true
	default:
		return false
	}
}

func (s UpdateService) UpdateAllPackages(cli install.Options) ([]UpdateResult, error) {
	candidates, _, _, err := s.ListUpdateCandidates()
	if err != nil {
		return nil, err
	}

	return s.UpdateCandidates(candidates, cli)
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
