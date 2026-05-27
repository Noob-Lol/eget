package cache

import (
	"fmt"
	"os"
	"path"
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

type CacheScanOptions struct {
	Kinds []Kind
	Root  string
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

	if !looksLikeEgetCacheDir(clean) && !hasKnownCacheLayout(clean) && !isEmptyDir(clean) {
		return fmt.Errorf("refuse to mutate non-eget cache dir %q", cacheDir)
	}

	return nil
}

func looksLikeEgetCacheDir(cacheDir string) bool {
	base := strings.ToLower(filepath.Base(filepath.Clean(cacheDir)))
	return base == "eget" || strings.Contains(base, "eget-cache") || strings.Contains(base, "eget_cache")
}

func hasKnownCacheLayout(cacheDir string) bool {
	for _, name := range []string{"api-cache", "sdk-downloads", "sdk-index"} {
		if info, err := os.Stat(filepath.Join(cacheDir, name)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func isEmptyDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) == 0
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

func (s Service) Scan(cacheDir string, opts CacheScanOptions) ([]Entry, error) {
	if cacheDir == "" {
		resolved, err := s.ResolveCacheDir()
		if err != nil {
			return nil, err
		}
		cacheDir = resolved
	}

	selected := cacheKindSet(normalizeScanKinds(opts.Kinds))
	root := strings.TrimSpace(opts.Root)

	var entries []Entry
	err := filepath.WalkDir(cacheDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == cacheDir || d.IsDir() {
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.Mode().IsDir() || (!info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0) {
			return nil
		}
		if err := ensurePathInDir(cacheDir, path); err != nil {
			return nil
		}

		rel, relErr := filepath.Rel(cacheDir, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		kind, partial := classifyEntry(rel)
		if !selected[kind] {
			return nil
		}
		if root != "" && !cacheRootAllows(root, kind) {
			return nil
		}

		entries = append(entries, Entry{
			Kind:      kind,
			Path:      path,
			RelPath:   rel,
			Size:      info.Size(),
			ModTime:   info.ModTime(),
			IsPartial: partial,
		})
		return nil
	})
	return entries, err
}

func (s Service) PreviewClean(cacheDir string, opts CleanOptions) (CleanResult, error) {
	opts.DryRun = true
	return s.clean(cacheDir, opts)
}

func (s Service) Clean(cacheDir string, opts CleanOptions) (CleanResult, error) {
	opts.DryRun = false
	return s.clean(cacheDir, opts)
}

func (s Service) clean(cacheDir string, opts CleanOptions) (CleanResult, error) {
	if cacheDir == "" {
		resolved, err := s.ResolveCacheDir()
		if err != nil {
			return CleanResult{}, err
		}
		cacheDir = resolved
	}
	if err := validateCacheDirForMutation(cacheDir); err != nil {
		return CleanResult{}, err
	}
	if opts.Older == 0 {
		opts.Older = 3 * 24 * time.Hour
	}

	entries, err := s.Scan(cacheDir, CacheScanOptions{Kinds: normalizeKinds(opts.Kinds)})
	if err != nil {
		return CleanResult{}, err
	}

	cutoff := s.now().Add(-opts.Older)
	result := CleanResult{CacheDir: cacheDir}
	for _, entry := range entries {
		if !opts.All && !entry.ModTime.Before(cutoff) {
			continue
		}
		result.MatchedFiles++
		result.MatchedSize += entry.Size
		if opts.DryRun {
			continue
		}
		if err := ensurePathInDir(cacheDir, entry.Path); err != nil {
			result.Skipped = append(result.Skipped, CleanSkip{Path: entry.Path, Reason: err.Error()})
			continue
		}
		if err := os.Remove(entry.Path); err != nil {
			result.Skipped = append(result.Skipped, CleanSkip{Path: entry.Path, Reason: err.Error()})
			continue
		}
		result.RemovedFiles++
		result.RemovedSize += entry.Size
		removeEmptyParents(cacheDir, filepath.Dir(entry.Path))
	}
	return result, nil
}

func normalizeScanKinds(kinds []Kind) []Kind {
	if len(kinds) == 0 {
		return []Kind{KindPkg, KindAPI, KindSDK, KindSDKIndex, KindPartial}
	}
	return dedupeKinds(kinds)
}

func normalizeKinds(kinds []Kind) []Kind {
	if len(kinds) == 0 {
		return append([]Kind(nil), defaultCacheCleanKinds...)
	}
	return dedupeKinds(kinds)
}

func dedupeKinds(kinds []Kind) []Kind {
	seen := map[Kind]bool{}
	out := make([]Kind, 0, len(kinds))
	for _, kind := range kinds {
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	return out
}

func cacheKindSet(kinds []Kind) map[Kind]bool {
	set := make(map[Kind]bool, len(kinds))
	for _, kind := range kinds {
		set[kind] = true
	}
	return set
}

func classifyEntry(rel string) (Kind, bool) {
	base := path.Base(rel)
	if strings.HasSuffix(base, ".part") || strings.HasSuffix(base, ".meta.json") {
		return KindPartial, true
	}

	switch {
	case strings.HasPrefix(rel, "api-cache/"):
		return KindAPI, false
	case strings.HasPrefix(rel, "sdk-downloads/"):
		return KindSDK, false
	case strings.HasPrefix(rel, "sdk-index/"):
		return KindSDKIndex, false
	default:
		return KindPkg, false
	}
}

func cacheRootAllows(root string, kind Kind) bool {
	switch strings.TrimSpace(root) {
	case "", "all":
		return kind != KindPartial
	case "pkg":
		return kind == KindPkg
	case "api":
		return kind == KindAPI
	case "sdk":
		return kind == KindSDK
	case "sdk-index":
		return kind == KindSDKIndex
	default:
		return false
	}
}

func ValidRoot(root string) bool {
	switch strings.TrimSpace(root) {
	case "", "all", "pkg", "api", "sdk", "sdk-index":
		return true
	default:
		return false
	}
}

func removeEmptyParents(root, dir string) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return
	}

	for {
		dirAbs, err := filepath.Abs(dir)
		if err != nil || filepath.Clean(dirAbs) == filepath.Clean(rootAbs) {
			return
		}
		if ensurePathInDir(rootAbs, dirAbs) != nil {
			return
		}
		if err := os.Remove(dirAbs); err != nil {
			return
		}
		dir = filepath.Dir(dirAbs)
	}
}
