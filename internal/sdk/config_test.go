package sdk

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

func TestResolveConfigUsesSDKNameAndAliases(t *testing.T) {
	file := cfgpkg.NewFile()
	file.Global.SDKTarget = stringPtr("~/sdks")
	file.SDK["go"] = cfgpkg.SDKSection{
		Aliases:         []string{"golang"},
		Target:          stringPtr("gosdk/go{version}"),
		URLTemplate:     stringPtr("https://example.com/go{version}.{os}-{arch}.{ext}"),
		IndexURL:        stringPtr("https://example.com/golang/"),
		IndexFormat:     stringPtr("html"),
		FilenamePattern: stringPtr("go{version}.{os}-{arch}.{ext}"),
		StripComponents: intPtr(1),
	}

	got, err := ResolveConfig(file, "golang", ResolveConfigOptions{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}

	assert.Eq(t, "go", got.Name)
	assert.Eq(t, "~/sdks", got.SDKTarget)
	assert.Eq(t, "gosdk/go{version}", got.TargetTemplate)
	assert.Eq(t, "tar.gz", got.Ext)
	assert.Eq(t, "linux", got.OS)
	assert.Eq(t, "amd64", got.Arch)
	assert.Eq(t, 1, got.StripComponents)
}

func TestResolveConfigUsesDefaultGlobalSDKExtMap(t *testing.T) {
	file := cfgpkg.NewFile()
	file.Global.SDKTarget = stringPtr("~/sdks")
	file.SDK["go"] = cfgpkg.SDKSection{
		Target:          stringPtr("gosdk/go{version}"),
		URLTemplate:     stringPtr("https://example.com/go{version}.{os}-{arch}.{ext}"),
		FilenamePattern: stringPtr("go{version}.{os}-{arch}.{ext}"),
	}

	got, err := ResolveConfig(file, "go", ResolveConfigOptions{GOOS: "windows", GOARCH: "amd64"})
	assert.NoErr(t, err)
	assert.Eq(t, "zip", got.Ext)
}

func TestResolveConfigUsesPlatformMapsAndSDKExtOverride(t *testing.T) {
	file := cfgpkg.NewFile()
	file.Global.SDKTarget = stringPtr("~/sdks")
	file.Global.SDKExtMap = map[string]string{"windows": "zip"}
	file.SDK["node"] = cfgpkg.SDKSection{
		Target:          stringPtr("nodejs/node{version}"),
		URLTemplate:     stringPtr("https://example.com/node-v{version}-{os}-{arch}.{ext}"),
		OSMap:           map[string]string{"windows": "win"},
		ArchMap:         map[string]string{"amd64": "x64"},
		ExtMap:          map[string]string{"windows": "7z"},
		FilenamePattern: stringPtr("node-v{version}-{os}-{arch}.{ext}"),
	}

	got, err := ResolveConfig(file, "node", ResolveConfigOptions{GOOS: "windows", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}

	assert.Eq(t, "win", got.OS)
	assert.Eq(t, "x64", got.Arch)
	assert.Eq(t, "7z", got.Ext)
}

func TestResolveConfigRequiresTarget(t *testing.T) {
	file := cfgpkg.NewFile()
	file.Global.SDKTarget = stringPtr("~/sdks")
	file.SDK["go"] = cfgpkg.SDKSection{}

	_, err := ResolveConfig(file, "go", ResolveConfigOptions{GOOS: "linux", GOARCH: "amd64"})
	assert.Err(t, err)
}

func stringPtr(v string) *string {
	return &v
}

func intPtr(v int) *int {
	return &v
}
