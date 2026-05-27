package cache

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/util"
)

type Service struct {
	Config *cfgpkg.File
	Now    func() time.Time
}

func ParseOlderDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("older duration is required")
	}

	unit := value[len(value)-1]
	if unit == 'd' || unit == 'w' {
		n, err := strconv.Atoi(value[:len(value)-1])
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid older duration %q", value)
		}
		if unit == 'd' {
			return time.Duration(n) * 24 * time.Hour, nil
		}
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}

	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid older duration %q", value)
	}
	return d, nil
}

func (s Service) ResolveCacheDir() (string, error) {
	cacheDir := "~/.cache/eget"
	if s.Config != nil && s.Config.Global.CacheDir != nil && strings.TrimSpace(*s.Config.Global.CacheDir) != "" {
		cacheDir = *s.Config.Global.CacheDir
	}

	expanded, err := util.Expand(cacheDir)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(expanded) == "" {
		return "", fmt.Errorf("cache dir is empty")
	}
	return filepath.Abs(expanded)
}

func validateCacheDirForMutation(cacheDir string) error {
	cacheDir = strings.TrimSpace(cacheDir)
	if cacheDir == "" {
		return fmt.Errorf("cache dir is empty")
	}

	abs, err := filepath.Abs(cacheDir)
	if err != nil {
		return err
	}

	clean := filepath.Clean(abs)
	volumeRoot := filepath.VolumeName(clean) + string(filepath.Separator)
	if clean == filepath.Clean(volumeRoot) {
		return fmt.Errorf("refuse to mutate dangerous cache dir %q", cacheDir)
	}

	home, err := util.Home()
	if err == nil {
		homeAbs, homeErr := filepath.Abs(home)
		if homeErr == nil && filepath.Clean(homeAbs) == clean {
			return fmt.Errorf("refuse to mutate home directory %q", cacheDir)
		}
	}

	return nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func ensurePathInDir(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q is outside cache dir", path)
	}
	return nil
}
