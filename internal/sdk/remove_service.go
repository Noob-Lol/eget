package sdk

import (
	"fmt"
	"os"
)

func (s Service) Remove(rawTarget string) (RemoveResult, error) {
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return RemoveResult{}, err
	}
	if target.Kind == VersionLatest {
		return RemoveResult{}, fmt.Errorf("sdk remove requires an explicit version")
	}
	cfg, err := s.resolveConfig(target.Name)
	if err != nil {
		return RemoveResult{}, err
	}
	entry, err := s.Store.Remove(cfg.Name, target.Version)
	if err != nil {
		return RemoveResult{}, err
	}
	if err := s.ensureSafeSDKPath(entry.Path, cfg); err != nil {
		_ = s.Store.Record(entry)
		return RemoveResult{}, err
	}
	err = os.RemoveAll(entry.Path)
	missing := false
	if os.IsNotExist(err) {
		missing = true
		err = nil
	}
	if err != nil {
		_ = s.Store.Record(entry)
		return RemoveResult{}, err
	}
	return RemoveResult{Name: cfg.Name, Version: target.Version, Path: entry.Path, Missing: missing}, nil
}
