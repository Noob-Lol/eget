package sdk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	cfgpkg "github.com/inherelab/eget/internal/config"
)

type Store struct {
	Path string
}

func DefaultStorePath() (string, error) {
	cfgPath, err := cfgpkg.ResolveWritablePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), "sdk.installed.json"), nil
}

func (s Store) Load() (InstalledStore, error) {
	if s.Path == "" {
		return newInstalledStore(), nil
	}
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return newInstalledStore(), nil
	}
	if err != nil {
		return InstalledStore{}, err
	}
	store := newInstalledStore()
	if err := json.Unmarshal(data, &store); err != nil {
		return InstalledStore{}, err
	}
	normalizeInstalledStore(&store)
	return store, nil
}

func (s Store) Save(store InstalledStore) error {
	normalizeInstalledStore(&store)
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.Path, append(data, '\n'), 0o644)
}

func (s Store) Record(entry InstalledEntry) error {
	store, err := s.Load()
	if err != nil {
		return err
	}
	node := store.Installed[entry.Name]
	if node.Versions == nil {
		node.Versions = map[string]InstalledEntry{}
	}
	node.Versions[entry.Version] = entry
	store.Installed[entry.Name] = node
	return s.Save(store)
}

func (s Store) Remove(name, version string) (InstalledEntry, error) {
	store, err := s.Load()
	if err != nil {
		return InstalledEntry{}, err
	}
	node, ok := store.Installed[name]
	if !ok {
		return InstalledEntry{}, fmt.Errorf("sdk %s is not installed", name)
	}
	entry, ok := node.Versions[version]
	if !ok {
		return InstalledEntry{}, fmt.Errorf("sdk %s@%s is not installed", name, version)
	}
	delete(node.Versions, version)
	if len(node.Versions) == 0 {
		delete(store.Installed, name)
	} else {
		store.Installed[name] = node
	}
	if err := s.Save(store); err != nil {
		return InstalledEntry{}, err
	}
	return entry, nil
}

func (s Store) List(name string) ([]InstalledEntry, error) {
	store, err := s.Load()
	if err != nil {
		return nil, err
	}
	var entries []InstalledEntry
	for sdkName, node := range store.Installed {
		if name != "" && sdkName != name {
			continue
		}
		for _, entry := range node.Versions {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return compareVersion(entries[i].Version, entries[j].Version) < 0
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func newInstalledStore() InstalledStore {
	return InstalledStore{
		Schema:    1,
		Installed: map[string]InstalledSDKNode{},
	}
}

func normalizeInstalledStore(store *InstalledStore) {
	if store.Schema == 0 {
		store.Schema = 1
	}
	if store.Installed == nil {
		store.Installed = map[string]InstalledSDKNode{}
	}
	for name, node := range store.Installed {
		if node.Versions == nil {
			node.Versions = map[string]InstalledEntry{}
			store.Installed[name] = node
		}
	}
}
