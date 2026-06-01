package sdk

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

func (s Service) RefreshIndex(ctx context.Context, name string) (Index, error) {
	cfg, err := s.resolveConfig(name)
	if err != nil {
		return Index{}, err
	}
	if cfg.IndexURL == "" {
		return Index{}, fmt.Errorf("sdk %s index_url is not configured", cfg.Name)
	}
	s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshFetchStart, SDK: cfg.Name, URL: cfg.IndexURL})
	body, err := s.fetchIndex(ctx, cfg.IndexURL)
	if err != nil {
		if cached, loadErr := s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL); loadErr == nil {
			s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshCacheHit, SDK: cfg.Name, URL: cfg.IndexURL, Err: err, Versions: len(cached.Items), Files: countIndexFiles(cached)})
			return cached, nil
		}
		return Index{}, err
	}
	defer body.Close()
	s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshFetchDone, SDK: cfg.Name, URL: cfg.IndexURL})

	var index Index
	format := cfg.IndexFormat
	parser := cfg.IndexParser
	switch {
	case cfg.IndexParser != "":
		if format == "" {
			format = "json"
		}
		s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseStart, SDK: cfg.Name, URL: cfg.IndexURL, Format: format, Parser: parser})
		index, err = ParseJSONIndex(body, cfg.IndexParser, JSONParseOptions{SDK: cfg.Name, SourceURL: cfg.IndexURL, Now: s.Now})
	case cfg.IndexFormat == "json":
		if parser == "" {
			parser = cfg.Name + "-json"
		}
		s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseStart, SDK: cfg.Name, URL: cfg.IndexURL, Format: "json", Parser: parser})
		index, err = ParseJSONIndex(body, cfg.IndexParser, JSONParseOptions{SDK: cfg.Name, SourceURL: cfg.IndexURL, Now: s.Now})
	default:
		if format == "" {
			format = "html"
		}
		s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseStart, SDK: cfg.Name, URL: cfg.IndexURL, Format: format})
		index, err = ParseHTMLIndex(body, HTMLParseOptions{
			SDK:             cfg.Name,
			SourceURL:       cfg.IndexURL,
			IndexPathPrefix: cfg.IndexPathPrefix,
			FilenamePattern: cfg.FilenamePattern,
			URLTemplate:     cfg.URLTemplate,
			OS:              cfg.OS,
			Arch:            cfg.Arch,
			Ext:             cfg.Ext,
			Now:             s.Now,
		})
	}
	if err != nil {
		s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseFailed, SDK: cfg.Name, URL: cfg.IndexURL, Format: format, Parser: parser, Err: err})
		return Index{}, err
	}
	s.emitIndexRefresh(IndexRefreshEvent{Stage: IndexRefreshParseDone, SDK: cfg.Name, URL: cfg.IndexURL, Format: format, Parser: parser, Versions: len(index.Items), Files: countIndexFiles(index)})
	if err := s.IndexCache.Save(index); err != nil {
		return Index{}, err
	}
	return index, nil
}

func (s Service) RefreshAllIndexes(ctx context.Context) ([]Index, error) {
	if s.Config == nil {
		return nil, nil
	}
	names := make([]string, 0, len(s.Config.SDK))
	for name, section := range s.Config.SDK {
		if section.IndexURL != nil && *section.IndexURL != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	indexes := make([]Index, 0, len(names))
	for _, name := range names {
		index, err := s.RefreshIndex(ctx, name)
		if err != nil {
			return nil, err
		}
		indexes = append(indexes, index)
	}
	return indexes, nil
}

func (s Service) ShowIndex(name string) (Index, error) {
	cfg, err := s.resolveConfig(name)
	if err != nil {
		return Index{}, err
	}
	return s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL)
}

func (s Service) ListIndexes() ([]CachedIndexInfo, error) {
	if s.Config == nil || len(s.Config.SDK) == 0 {
		return []CachedIndexInfo{}, nil
	}
	names := make([]string, 0, len(s.Config.SDK))
	for name, section := range s.Config.SDK {
		if section.IndexURL == nil || strings.TrimSpace(*section.IndexURL) == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	infos := make([]CachedIndexInfo, 0, len(names))
	for _, name := range names {
		cfg, err := s.resolveConfig(name)
		if err != nil {
			return nil, err
		}
		info := CachedIndexInfo{
			SDK:       cfg.Name,
			SourceURL: cfg.IndexURL,
			Path:      s.IndexCache.PathForSource(cfg.Name, cfg.IndexURL),
		}
		index, err := s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL)
		if err == nil {
			info.Versions = len(index.Items)
			info.SourceURL = index.SourceURL
			info.FetchedAt = index.FetchedAt
			info.Path = s.IndexCache.PathForSource(index.SDK, index.SourceURL)
			info.Cached = true
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (s Service) ClearIndex(name string) error {
	cfg, err := s.resolveConfig(name)
	if err != nil {
		return err
	}
	return s.IndexCache.ClearForSource(cfg.Name, cfg.IndexURL)
}

func (s Service) ClearAllIndexes() error {
	return s.IndexCache.ClearAll()
}

func (s Service) emitIndexRefresh(event IndexRefreshEvent) {
	if s.OnIndexRefresh != nil {
		s.OnIndexRefresh(event)
	}
}

func countIndexFiles(index Index) int {
	total := 0
	for _, item := range index.Items {
		total += len(item.Files)
	}
	return total
}
