package sdk

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type IndexCache struct {
	Dir string
}

type CachedIndexInfo struct {
	SDK       string
	Versions  int
	SourceURL string
	FetchedAt time.Time
	Path      string
	Cached    bool
}

func (c IndexCache) Path(name string) string {
	return filepath.Join(c.Dir, safeName(name)+".json")
}

func (c IndexCache) PathForSource(name, sourceURL string) string {
	host := indexSourceHost(sourceURL)
	if host == "" {
		return c.Path(name)
	}
	return filepath.Join(c.Dir, safeName(name)+"-"+safeName(host)+".json")
}

func (c IndexCache) Load(name string) (Index, error) {
	return c.loadPath(c.Path(name))
}

func (c IndexCache) LoadForSource(name, sourceURL string) (Index, error) {
	index, err := c.loadPath(c.PathForSource(name, sourceURL))
	if err == nil || indexSourceHost(sourceURL) == "" || !os.IsNotExist(err) {
		return index, err
	}
	return c.Load(name)
}

func (c IndexCache) loadPath(path string) (Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Index{}, err
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, err
	}
	return index, nil
}

func (c IndexCache) Save(index Index) error {
	if index.Schema == 0 {
		index.Schema = 1
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.PathForSource(index.SDK, index.SourceURL), append(data, '\n'), 0o644)
}

func (c IndexCache) Clear(name string) error {
	err := os.Remove(c.Path(name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (c IndexCache) ClearForSource(name, sourceURL string) error {
	paths := []string{c.PathForSource(name, sourceURL)}
	legacyPath := c.Path(name)
	if legacyPath != paths[0] {
		paths = append(paths, legacyPath)
	}
	for _, path := range paths {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (c IndexCache) ClearAll() error {
	entries, err := os.ReadDir(c.Dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if err := os.Remove(filepath.Join(c.Dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (c IndexCache) List() ([]CachedIndexInfo, error) {
	entries, err := os.ReadDir(c.Dir)
	if os.IsNotExist(err) {
		return []CachedIndexInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	infos := make([]CachedIndexInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(c.Dir, entry.Name())
		index, err := c.loadPath(path)
		if err != nil {
			return nil, err
		}
		infos = append(infos, CachedIndexInfo{
			SDK:       index.SDK,
			Versions:  len(index.Items),
			SourceURL: index.SourceURL,
			FetchedAt: index.FetchedAt,
			Path:      path,
			Cached:    true,
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].SDK < infos[j].SDK
	})
	return infos, nil
}

func SelectVersion(index Index, target Target) (IndexItem, error) {
	var candidates []IndexItem
	for _, item := range index.Items {
		switch target.Kind {
		case VersionLatest:
			if item.Stable {
				candidates = append(candidates, item)
			}
		case VersionPrefix:
			if item.Stable && strings.HasPrefix(item.Version, target.Version+".") {
				candidates = append(candidates, item)
			}
		case VersionExact:
			if item.Version == target.Version {
				return item, nil
			}
		}
	}
	if len(candidates) == 0 {
		return IndexItem{}, fmt.Errorf("sdk %s version %s not found", target.Name, target.Version)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareVersion(candidates[i].Version, candidates[j].Version) > 0
	})
	return candidates[0], nil
}

func SelectFile(item IndexItem, osName, arch, ext string) (IndexFile, error) {
	for _, file := range item.Files {
		if file.OS == osName && file.Arch == arch && file.Ext == ext {
			return file, nil
		}
	}
	return IndexFile{}, fmt.Errorf("sdk file for %s/%s.%s not found", osName, arch, ext)
}

func safeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, string(os.PathSeparator), "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, ":", "-")
	return name
}

func indexSourceHost(sourceURL string) string {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return ""
	}
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
