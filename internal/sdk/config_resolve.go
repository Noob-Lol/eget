package sdk

import (
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/inherelab/eget/internal/client"
	"github.com/inherelab/eget/internal/util"
)

func (s Service) resolveConfig(name string) (Config, error) {
	return s.resolveConfigForPlatform(name, PlatformOptions{})
}

func (s Service) resolveConfigForPlatform(name string, platform PlatformOptions) (Config, error) {
	goos := platform.OS
	if goos == "" {
		goos = s.GOOS
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := platform.Arch
	if goarch == "" {
		goarch = s.GOARCH
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return ResolveConfig(s.Config, name, ResolveConfigOptions{GOOS: goos, GOARCH: goarch})
}

func (s Service) effectiveClientOptions() client.Options {
	opts := s.ClientOpts
	if opts.UserAgent == "" && s.Config != nil && s.Config.Global.UserAgent != nil {
		opts.UserAgent = *s.Config.Global.UserAgent
	}
	return opts
}

func (s Service) resolveVersionAndFile(target Target, cfg Config) (string, IndexFile, error) {
	if target.Kind == VersionExact && cfg.URLTemplate != "" {
		return target.Version, IndexFile{}, nil
	}
	index, err := s.IndexCache.LoadForSource(cfg.Name, cfg.IndexURL)
	if err != nil {
		return "", IndexFile{}, err
	}
	item, err := SelectVersion(index, target)
	if err != nil {
		return "", IndexFile{}, err
	}
	file, err := SelectFile(item, cfg.OS, cfg.Arch, cfg.Ext)
	if err != nil {
		return "", IndexFile{}, err
	}
	return item.Version, file, nil
}

func (s Service) resolveInstallPath(cfg Config, vars TemplateVars) (string, error) {
	rendered, err := RenderTemplate(cfg.TargetTemplate, vars)
	if err != nil {
		return "", err
	}
	if expanded, err := util.Expand(rendered); err == nil && expanded != "" {
		rendered = expanded
	}
	if filepath.IsAbs(rendered) {
		return filepath.Clean(rendered), nil
	}
	return filepath.Join(s.sdkRoot(cfg), filepath.FromSlash(rendered)), nil
}

func (s Service) sdkRoot(cfg Config) string {
	root := "~/.local/sdks"
	if cfg.SDKTarget != "" {
		root = cfg.SDKTarget
	}
	expanded, err := util.Expand(root)
	if err == nil && expanded != "" {
		root = expanded
	}
	return filepath.Clean(root)
}

func (s Service) sdkBasePath(cfg Config) (string, error) {
	prefix := targetTemplateBasePrefix(cfg.TargetTemplate)
	if prefix == "" {
		return s.sdkRoot(cfg), nil
	}
	if expanded, err := util.Expand(prefix); err == nil && expanded != "" {
		prefix = expanded
	}
	if filepath.IsAbs(prefix) {
		return filepath.Clean(prefix), nil
	}
	return filepath.Join(s.sdkRoot(cfg), filepath.FromSlash(prefix)), nil
}

func targetTemplateBasePrefix(template string) string {
	before, _, _ := strings.Cut(template, "{version}")
	before = strings.TrimRight(before, `/\`)
	if before == "" || !strings.ContainsAny(before, `/\`) {
		return ""
	}
	return filepath.Dir(filepath.FromSlash(before))
}

func (s Service) cacheDir() string {
	if s.IndexCache.Dir != "" {
		return filepath.Dir(s.IndexCache.Dir)
	}
	return "."
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
