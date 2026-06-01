package sdk

import (
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/util"
)

type BuiltinConfigSource string

const (
	BuiltinConfigOfficial BuiltinConfigSource = "official"
	BuiltinConfigMirror   BuiltinConfigSource = "mirror"
	BuiltinConfigAliyun   BuiltinConfigSource = "aliyun"
	BuiltinConfigHuawei   BuiltinConfigSource = "huawei"
	BuiltinConfigZulu     BuiltinConfigSource = "zulu"
)

type BuiltinConfig struct {
	Name    string
	Aliases []string
	Source  BuiltinConfigSource
	Section cfgpkg.SDKSection
}

func BuiltinConfigs(source BuiltinConfigSource) []BuiltinConfig {
	switch source {
	case BuiltinConfigOfficial:
		return cloneBuiltinConfigs(builtinOfficialConfigs())
	case BuiltinConfigMirror:
		return cloneBuiltinConfigs(builtinMirrorConfigs())
	case BuiltinConfigAliyun:
		return cloneBuiltinConfigs(builtinAliyunConfigs())
	case BuiltinConfigHuawei:
		return cloneBuiltinConfigs(builtinHuaweiConfigs())
	case BuiltinConfigZulu:
		return cloneBuiltinConfigs(builtinZuluConfigs())
	default:
		return nil
	}
}

func FindBuiltinConfig(name string, source BuiltinConfigSource) (BuiltinConfig, bool) {
	for _, item := range BuiltinConfigs(source) {
		if item.Name == name {
			return item, true
		}
		for _, alias := range item.Aliases {
			if alias == name {
				return item, true
			}
		}
	}
	return BuiltinConfig{}, false
}

func BuiltinConfigNames() []string {
	return []string{"go", "node", "jdk"}
}

func BuiltinMirrorNames() []string {
	return []string{string(BuiltinConfigMirror), string(BuiltinConfigAliyun), string(BuiltinConfigHuawei), string(BuiltinConfigZulu)}
}

func builtinOfficialConfigs() []BuiltinConfig {
	return []BuiltinConfig{
		{
			Name:    "go",
			Aliases: []string{"golang"},
			Source:  BuiltinConfigOfficial,
			Section: cfgpkg.SDKSection{
				Aliases:         []string{"golang"},
				Target:          sdkStringPtr("gosdk/go{version}"),
				URLTemplate:     sdkStringPtr("https://go.dev/dl/go{version}.{os}-{arch}.{ext}"),
				IndexURL:        sdkStringPtr("https://go.dev/dl/"),
				IndexFormat:     sdkStringPtr("html"),
				FilenamePattern: sdkStringPtr("go{version}.{os}-{arch}.{ext}"),
				StripComponents: sdkIntPtr(1),
				ExtMap:          map[string]string{"windows": "zip", "linux": "tar.gz", "darwin": "tar.gz"},
			},
		},
		{
			Name:    "node",
			Aliases: []string{"nodejs"},
			Source:  BuiltinConfigOfficial,
			Section: cfgpkg.SDKSection{
				Aliases:         []string{"nodejs"},
				Target:          sdkStringPtr("nodejs/node{version}"),
				URLTemplate:     sdkStringPtr("https://nodejs.org/dist/v{version}/node-v{version}-{os}-{arch}.{ext}"),
				IndexURL:        sdkStringPtr("https://nodejs.org/dist/"),
				IndexFormat:     sdkStringPtr("html"),
				FilenamePattern: sdkStringPtr("node-v{version}-{os}-{arch}.{ext}"),
				StripComponents: sdkIntPtr(1),
				OSMap:           map[string]string{"windows": "win", "linux": "linux", "darwin": "darwin"},
				ArchMap:         map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"},
				ExtMap:          map[string]string{"windows": "zip", "linux": "tar.xz", "darwin": "tar.gz"},
			},
		},
		{
			Name:    "jdk",
			Aliases: []string{"java"},
			Source:  BuiltinConfigOfficial,
			Section: cfgpkg.SDKSection{
				Aliases:         []string{"java"},
				Target:          sdkStringPtr("jdk/openjdk-{version}"),
				IndexURL:        sdkStringPtr("https://jdk.java.net/archive/"),
				IndexFormat:     sdkStringPtr("html"),
				FilenamePattern: sdkStringPtr("openjdk-{version}_{os}-{arch}_bin.{ext}"),
				StripComponents: sdkIntPtr(1),
				ArchMap:         map[string]string{"amd64": "x64", "arm64": "aarch64"},
				OSMap:           map[string]string{"darwin": "macos"},
				ExtMap:          map[string]string{"windows": "zip", "linux": "tar.gz", "darwin": "tar.gz"},
			},
		},
	}
}

func builtinMirrorConfigs() []BuiltinConfig {
	items := append(builtinAliyunConfigs(), builtinHuaweiConfigs()...)
	for i := range items {
		items[i].Source = BuiltinConfigMirror
	}
	return items
}

func builtinAliyunConfigs() []BuiltinConfig {
	items := builtinOfficialConfigs()[:2]
	items[0].Source = BuiltinConfigAliyun
	items[0].Section.URLTemplate = sdkStringPtr("https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}")
	items[0].Section.IndexURL = sdkStringPtr("https://mirrors.aliyun.com/golang/")

	items[1].Source = BuiltinConfigAliyun
	items[1].Section.URLTemplate = sdkStringPtr("https://mirrors.aliyun.com/nodejs-release/v{version}/node-v{version}-{os}-{arch}.{ext}")
	items[1].Section.IndexURL = sdkStringPtr("https://mirrors.aliyun.com/nodejs-release/")
	items[1].Section.IndexPathPrefix = sdkStringPtr("/nodejs-release/")

	return items
}

func builtinHuaweiConfigs() []BuiltinConfig {
	item := builtinOfficialConfigs()[2]
	item.Source = BuiltinConfigHuawei
	item.Section.URLTemplate = sdkStringPtr("https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}")
	item.Section.IndexURL = sdkStringPtr("https://mirrors.huaweicloud.com/openjdk/")

	return []BuiltinConfig{item}
}

func builtinZuluConfigs() []BuiltinConfig {
	item := builtinOfficialConfigs()[2]
	item.Source = BuiltinConfigZulu
	item.Section.Target = sdkStringPtr("jdk/zulu-{version}")
	item.Section.URLTemplate = nil
	item.Section.IndexURL = sdkStringPtr("https://api.azul.com/metadata/v1/zulu/packages?java_package_type=jdk&release_status=ga&availability_type=CA")
	item.Section.IndexFormat = sdkStringPtr("json")
	item.Section.IndexParser = sdkStringPtr("zulu-json")
	item.Section.FilenamePattern = nil
	item.Section.OSMap = map[string]string{"windows": "win", "darwin": "macosx"}
	item.Section.ArchMap = map[string]string{"amd64": "x64", "arm64": "aarch64"}
	item.Section.ExtMap = map[string]string{"windows": "zip", "linux": "tar.gz", "darwin": "tar.gz"}

	return []BuiltinConfig{item}
}

func cloneBuiltinConfigs(items []BuiltinConfig) []BuiltinConfig {
	cloned := make([]BuiltinConfig, len(items))
	for i, item := range items {
		cloned[i] = BuiltinConfig{
			Name:    item.Name,
			Aliases: append([]string(nil), item.Aliases...),
			Source:  item.Source,
			Section: cloneSDKSection(item.Section),
		}
	}
	return cloned
}

func cloneSDKSection(section cfgpkg.SDKSection) cfgpkg.SDKSection {
	return cfgpkg.SDKSection{
		Aliases:         append([]string(nil), section.Aliases...),
		Target:          cloneBuiltinStringPtr(section.Target),
		URLTemplate:     cloneBuiltinStringPtr(section.URLTemplate),
		IndexURL:        cloneBuiltinStringPtr(section.IndexURL),
		IndexFormat:     cloneBuiltinStringPtr(section.IndexFormat),
		IndexParser:     cloneBuiltinStringPtr(section.IndexParser),
		IndexPathPrefix: cloneBuiltinStringPtr(section.IndexPathPrefix),
		FilenamePattern: cloneBuiltinStringPtr(section.FilenamePattern),
		StripComponents: cloneBuiltinIntPtr(section.StripComponents),
		OSMap:           util.CloneStringMap(section.OSMap),
		ArchMap:         util.CloneStringMap(section.ArchMap),
		ExtMap:          util.CloneStringMap(section.ExtMap),
	}
}

func cloneBuiltinStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBuiltinIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func sdkStringPtr(value string) *string {
	return &value
}

func sdkIntPtr(value int) *int {
	return &value
}
