package sdk

import (
	"fmt"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/util"
)

type Config struct {
	Name            string
	Aliases         []string
	SDKTarget       string
	TargetTemplate  string
	URLTemplate     string
	IndexURL        string
	IndexFormat     string
	IndexParser     string
	IndexPathPrefix string
	FilenamePattern string
	StripComponents int
	OS              string
	Arch            string
	Ext             string
	OSMap           map[string]string
	ArchMap         map[string]string
	ExtMap          map[string]string
}

type ResolveConfigOptions struct {
	GOOS   string
	GOARCH string
}

func ResolveConfig(file *cfgpkg.File, name string, opts ResolveConfigOptions) (Config, error) {
	if file == nil {
		file = cfgpkg.NewFile()
	}
	sdkName, section, ok := findSDKSection(file, name)
	if !ok {
		return Config{}, fmt.Errorf("sdk %q is not configured", name)
	}

	target := util.DerefString(section.Target)
	if target == "" {
		return Config{}, fmt.Errorf("sdk %q target is required", sdkName)
	}

	rawOS := opts.GOOS
	rawArch := opts.GOARCH
	osName := mappedValue(section.OSMap, rawOS, rawOS)
	arch := mappedValue(section.ArchMap, rawArch, rawArch)
	ext := mappedValue(section.ExtMap, rawOS, "")
	if ext == "" {
		ext = mappedValue(file.Global.SDKExtMap, rawOS, "")
	}
	if ext == "" {
		return Config{}, fmt.Errorf("sdk %q extension for %s is not configured", sdkName, rawOS)
	}

	return Config{
		Name:            sdkName,
		Aliases:         append([]string(nil), section.Aliases...),
		SDKTarget:       util.DerefString(file.Global.SDKTarget),
		TargetTemplate:  target,
		URLTemplate:     util.DerefString(section.URLTemplate),
		IndexURL:        util.DerefString(section.IndexURL),
		IndexFormat:     util.DerefString(section.IndexFormat),
		IndexParser:     util.DerefString(section.IndexParser),
		IndexPathPrefix: util.DerefString(section.IndexPathPrefix),
		FilenamePattern: util.DerefString(section.FilenamePattern),
		StripComponents: derefInt(section.StripComponents),
		OS:              osName,
		Arch:            arch,
		Ext:             ext,
		OSMap:           cloneStringMap(section.OSMap),
		ArchMap:         cloneStringMap(section.ArchMap),
		ExtMap:          cloneStringMap(section.ExtMap),
	}, nil
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func findSDKSection(file *cfgpkg.File, name string) (string, cfgpkg.SDKSection, bool) {
	if section, ok := file.SDK[name]; ok {
		return name, section, true
	}
	for sdkName, section := range file.SDK {
		for _, alias := range section.Aliases {
			if alias == name {
				return sdkName, section, true
			}
		}
	}
	return "", cfgpkg.SDKSection{}, false
}

func mappedValue(values map[string]string, key, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	if value := values[key]; value != "" {
		return value
	}
	return fallback
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}
