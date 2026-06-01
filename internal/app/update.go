package app

import (
	"fmt"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	storepkg "github.com/inherelab/eget/internal/installed"
)

type Installer interface {
	InstallTarget(target string, opts install.Options, extras ...InstallExtras) (RunResult, error)
}

type UpdateService struct {
	Install       Installer
	LoadConfig    func() (*cfgpkg.File, error)
	LoadInstalled func() (*storepkg.Config, error)
	LatestInfo    LatestInfoFunc
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
	opts.Operation = install.OperationUpdate
	opts.CurrentVersion = item.InstalledTag
	opts.TargetVersion = check.outdated.LatestTag
	return s.Install.InstallTarget(target, opts)
}

func (s UpdateService) UpdateAllPackages(cli install.Options) ([]UpdateResult, error) {
	candidates, _, _, err := s.ListUpdateCandidates()
	if err != nil {
		return nil, err
	}

	return s.UpdateCandidates(candidates, cli)
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
