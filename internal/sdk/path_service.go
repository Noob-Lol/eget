package sdk

import (
	"fmt"
	"sort"
	"strings"
)

func (s Service) List(name string) ([]InstalledEntry, error) {
	return s.Store.List(name)
}

func (s Service) Path(rawTarget string) (InstalledEntry, error) {
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return InstalledEntry{}, err
	}
	cfg, err := s.resolveConfig(target.Name)
	if err != nil {
		return InstalledEntry{}, err
	}
	if target.Kind == VersionLatest {
		path, err := s.sdkBasePath(cfg)
		if err != nil {
			return InstalledEntry{}, err
		}
		return InstalledEntry{Name: cfg.Name, Path: path}, nil
	}
	entries, err := s.Store.List(cfg.Name)
	if err != nil {
		return InstalledEntry{}, err
	}
	entry, ok := selectInstalledSDKPath(target, cfg.Name, entries)
	if !ok {
		return InstalledEntry{}, fmt.Errorf("sdk %s version %s is not installed", cfg.Name, target.Version)
	}
	return entry, nil
}

func selectInstalledSDKPath(target Target, name string, entries []InstalledEntry) (InstalledEntry, bool) {
	candidates := make([]InstalledEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		switch target.Kind {
		case VersionExact:
			if entry.Version == target.Version {
				return entry, true
			}
		case VersionPrefix:
			if isStableVersion(entry.Version) && strings.HasPrefix(entry.Version, target.Version+".") {
				candidates = append(candidates, entry)
			}
		}
	}
	if len(candidates) == 0 {
		return InstalledEntry{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareVersion(candidates[i].Version, candidates[j].Version) > 0
	})
	return candidates[0], true
}
