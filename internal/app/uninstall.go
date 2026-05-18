package app

import (
	"fmt"
	"os"
	"strings"

	cfgpkg "github.com/inherelab/eget/internal/config"
	storepkg "github.com/inherelab/eget/internal/installed"
	"github.com/inherelab/eget/internal/util"
)

type RemovableInstalledStore interface {
	Load() (*storepkg.Config, error)
	Remove(target string) error
}

type UninstallResult struct {
	Repo         string
	RemovedFiles []string
}

type uninstallTarget struct {
	Key  string
	Repo string
}

type UninstallService struct {
	Store      RemovableInstalledStore
	LoadConfig func() (*cfgpkg.File, error)
}

func (s UninstallService) Uninstall(target string) (UninstallResult, error) {
	resolved, err := s.resolveTarget(target)
	if err != nil {
		return UninstallResult{}, err
	}
	if s.Store == nil {
		return UninstallResult{}, fmt.Errorf("installed store is required")
	}
	cfg, err := s.Store.Load()
	if err != nil {
		return UninstallResult{}, err
	}
	entry, key, ok := findUninstallEntry(cfg, resolved)
	if !ok {
		return UninstallResult{}, fmt.Errorf("installed entry not found for %q", resolved.Key)
	}

	result := UninstallResult{Repo: firstNonEmpty(entry.Repo, resolved.Repo)}
	for _, file := range entry.ExtractedFiles {
		err := os.Remove(file)
		if err != nil && !os.IsNotExist(err) {
			return UninstallResult{}, err
		}
		result.RemovedFiles = append(result.RemovedFiles, file)
	}
	if err := s.Store.Remove(key); err != nil {
		return UninstallResult{}, err
	}
	return result, nil
}

func (s UninstallService) resolveTarget(target string) (uninstallTarget, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return uninstallTarget{}, err
	}
	if pkg, ok := cfg.Packages[target]; ok {
		repo := util.DerefString(pkg.Repo)
		if repo == "" {
			return uninstallTarget{}, fmt.Errorf("package %q has no repo", target)
		}
		return uninstallTarget{Key: target, Repo: repo}, nil
	}
	if strings.Contains(target, "/") {
		return uninstallTarget{Key: target, Repo: target}, nil
	}
	return uninstallTarget{}, fmt.Errorf("unknown package %q", target)
}

func findUninstallEntry(cfg *storepkg.Config, target uninstallTarget) (storepkg.Entry, string, bool) {
	if cfg == nil || cfg.Installed == nil {
		return storepkg.Entry{}, "", false
	}
	for _, key := range uninstallCandidateKeys(target) {
		if entry, ok := cfg.Installed[key]; ok {
			return entry, key, true
		}
	}
	return storepkg.Entry{}, "", false
}

func uninstallCandidateKeys(target uninstallTarget) []string {
	keys := []string{target.Key}
	if normalized := storepkg.NormalizeRepoName(target.Key); normalized != target.Key {
		keys = append(keys, normalized)
	}
	if target.Repo != "" && target.Repo != target.Key {
		keys = append(keys, target.Repo)
	}
	if normalized := storepkg.NormalizeRepoName(target.Repo); normalized != "" && normalized != target.Repo {
		keys = append(keys, normalized)
	}
	return keys
}

func (s UninstallService) loadConfig() (*cfgpkg.File, error) {
	if s.LoadConfig != nil {
		return s.LoadConfig()
	}
	return cfgpkg.Load()
}
