package sdk

import (
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/util"
)

func TestServicePathReturnsConfiguredSDKBasePath(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	cfg.SDK["jdk"] = cfgpkg.SDKSection{
		Aliases: []string{"java"},
		Target:  stringPtr("jdk/openjdk-{version}"),
		ExtMap:  map[string]string{"linux": "tar.gz"},
	}
	svc := Service{Config: cfg, Store: Store{Path: filepath.Join(root, "sdk.installed.json")}, GOOS: "linux", GOARCH: "amd64"}

	goEntry, err := svc.Path("go")
	assert.NoErr(t, err)
	assert.Eq(t, filepath.Join(root, "sdks", "gosdk"), filepath.Clean(goEntry.Path))
	assert.Eq(t, "go", goEntry.Name)
	assert.Eq(t, "", goEntry.Version)

	javaEntry, err := svc.Path("java")
	assert.NoErr(t, err)
	assert.Eq(t, filepath.Join(root, "sdks", "jdk"), filepath.Clean(javaEntry.Path))
	assert.Eq(t, "jdk", javaEntry.Name)
	assert.Eq(t, "", javaEntry.Version)
}

func TestServicePathReturnsInstalledVersionPath(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	assert.NoErr(t, store.Record(InstalledEntry{Name: "go", Version: "1.20.3", Path: filepath.Join(root, "sdks", "gosdk", "go1.20.3")}))
	assert.NoErr(t, store.Record(InstalledEntry{Name: "go", Version: "1.20.12", Path: filepath.Join(root, "sdks", "gosdk", "go1.20.12")}))
	assert.NoErr(t, store.Record(InstalledEntry{Name: "go", Version: "1.21.5", Path: filepath.Join(root, "sdks", "gosdk", "go1.21.5")}))
	svc := Service{Config: cfg, Store: store, GOOS: "linux", GOARCH: "amd64"}

	prefix, err := svc.Path("go:1.20")
	assert.NoErr(t, err)
	assert.Eq(t, "1.20.12", prefix.Version)
	assert.Eq(t, filepath.Join(root, "sdks", "gosdk", "go1.20.12"), filepath.Clean(prefix.Path))

	exact, err := svc.Path("go@1.21.5")
	assert.NoErr(t, err)
	assert.Eq(t, "1.21.5", exact.Version)
	assert.Eq(t, filepath.Join(root, "sdks", "gosdk", "go1.21.5"), filepath.Clean(exact.Path))
}

func TestServicePathUsesSDKAliasForInstalledVersion(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	cfg.SDK["jdk"] = cfgpkg.SDKSection{
		Aliases: []string{"java"},
		Target:  stringPtr("jdk/zulu-{version}"),
		ExtMap:  map[string]string{"linux": "tar.gz"},
	}
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	assert.NoErr(t, store.Record(InstalledEntry{Name: "jdk", Version: "17.0.9", Path: filepath.Join(root, "sdks", "jdk", "zulu-17.0.9")}))
	assert.NoErr(t, store.Record(InstalledEntry{Name: "jdk", Version: "17.0.11", Path: filepath.Join(root, "sdks", "jdk", "zulu-17.0.11")}))
	svc := Service{Config: cfg, Store: store, GOOS: "linux", GOARCH: "amd64"}

	got, err := svc.Path("java:17")
	assert.NoErr(t, err)
	assert.Eq(t, "jdk", got.Name)
	assert.Eq(t, "17.0.11", got.Version)
	assert.Eq(t, filepath.Join(root, "sdks", "jdk", "zulu-17.0.11"), filepath.Clean(got.Path))
}

func TestServicePathReturnsErrorForMissingInstalledVersion(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	svc := Service{Config: cfg, Store: Store{Path: filepath.Join(root, "sdk.installed.json")}, GOOS: "linux", GOARCH: "amd64"}

	_, err := svc.Path("go:1.20")
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestServiceSDKRootExpandsTildeTarget(t *testing.T) {
	cfg := Config{SDKTarget: "~/.local/sdk"}
	svc := Service{}

	expected, err := util.Expand("~/.local/sdk")
	if err != nil {
		t.Fatalf("expand expected sdk target: %v", err)
	}

	assert.Eq(t, filepath.Clean(expected), svc.sdkRoot(cfg))
	assert.NotContains(t, svc.sdkRoot(cfg), "~")
}

func TestServiceSDKRootUsesDefaultWhenTargetMissingOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "missing", cfg: Config{}},
		{name: "empty", cfg: Config{SDKTarget: ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := Service{}

			expected, err := util.Expand("~/.local/sdks")
			assert.NoErr(t, err)
			assert.Eq(t, filepath.Clean(expected), svc.sdkRoot(tt.cfg))
			assert.NotContains(t, svc.sdkRoot(tt.cfg), "~")
		})
	}
}

func TestServiceResolveInstallPathExpandsTildeTargetTemplate(t *testing.T) {
	svc := Service{}
	cfg := Config{
		SDKTarget:      "~/.local/sdk",
		TargetTemplate: "~/app/jdk/zulu-{version}",
	}

	got, err := svc.resolveInstallPath(cfg, TemplateVars{Version: "21.0.11"})
	assert.NoErr(t, err)
	expected, err := util.Expand("~/app/jdk/zulu-21.0.11")
	assert.NoErr(t, err)
	assert.Eq(t, filepath.Clean(expected), got)
	assert.NotContains(t, got, "~")
	assert.NotContains(t, got, filepath.Join(".local", "sdk"))
}
