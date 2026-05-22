package app

import (
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

func TestAddSDKConfigAddsOfficialTemplate(t *testing.T) {
	cfg := cfgpkg.NewFile()
	saved := false
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error {
			saved = true
			return nil
		},
	}

	result, err := svc.AddSDKConfig(SDKConfigAddOptions{Name: "jdk"})
	assert.NoErr(t, err)
	assert.True(t, saved)
	assert.Eq(t, 1, len(result.Items))
	assert.Eq(t, "jdk", result.Items[0].Name)
	assert.Eq(t, SDKConfigActionAdded, result.Items[0].Action)
	assert.Eq(t, "https://jdk.java.net/archive/", *cfg.SDK["jdk"].IndexURL)
	assert.Nil(t, cfg.SDK["jdk"].URLTemplate)
}

func TestAddSDKConfigAddsMirrorTemplateByAlias(t *testing.T) {
	cfg := cfgpkg.NewFile()
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error { return nil },
	}

	result, err := svc.AddSDKConfig(SDKConfigAddOptions{Name: "java", Mirror: true})
	assert.NoErr(t, err)
	assert.Eq(t, "jdk", result.Items[0].Name)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *cfg.SDK["jdk"].IndexURL)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}", *cfg.SDK["jdk"].URLTemplate)
}

func TestAddSDKConfigRejectsExistingWithoutForce(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.SDK["jdk"] = cfgpkg.SDKSection{Target: appStringPtr("custom")}
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error {
			t.Fatal("save should not be called")
			return nil
		},
	}

	_, err := svc.AddSDKConfig(SDKConfigAddOptions{Name: "jdk"})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Eq(t, "custom", *cfg.SDK["jdk"].Target)
}

func TestAddSDKConfigForceUpdatesExisting(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.SDK["jdk"] = cfgpkg.SDKSection{Target: appStringPtr("custom")}
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error { return nil },
	}

	result, err := svc.AddSDKConfig(SDKConfigAddOptions{Name: "jdk", Force: true, Mirror: true})
	assert.NoErr(t, err)
	assert.Eq(t, SDKConfigActionUpdated, result.Items[0].Action)
	assert.Eq(t, "jdk/openjdk-{version}", *cfg.SDK["jdk"].Target)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *cfg.SDK["jdk"].IndexURL)
}

func TestAddSDKConfigAllSkipsExistingAndAddsMissing(t *testing.T) {
	cfg := cfgpkg.NewFile()
	cfg.SDK["go"] = cfgpkg.SDKSection{Target: appStringPtr("custom-go")}
	svc := ConfigService{
		Load: func() (*cfgpkg.File, error) { return cfg, nil },
		Save: func(path string, file *cfgpkg.File) error { return nil },
	}

	result, err := svc.AddSDKConfig(SDKConfigAddOptions{All: true, Mirror: true})
	assert.NoErr(t, err)
	assert.Eq(t, 3, len(result.Items))
	assert.Eq(t, SDKConfigActionSkipped, result.Items[0].Action)
	assert.Eq(t, "custom-go", *cfg.SDK["go"].Target)
	assert.Eq(t, "https://mirrors.aliyun.com/nodejs-release/", *cfg.SDK["node"].IndexURL)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *cfg.SDK["jdk"].IndexURL)
}

func TestAddSDKConfigValidatesInput(t *testing.T) {
	cfg := cfgpkg.NewFile()
	svc := ConfigService{Load: func() (*cfgpkg.File, error) { return cfg, nil }}

	_, err := svc.AddSDKConfig(SDKConfigAddOptions{})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "requires exactly one")

	_, err = svc.AddSDKConfig(SDKConfigAddOptions{Name: "jdk", All: true})
	assert.Err(t, err)
	assert.Contains(t, err.Error(), "requires exactly one")

	_, err = svc.AddSDKConfig(SDKConfigAddOptions{Name: "ruby"})
	assert.Err(t, err)
	assert.True(t, strings.Contains(err.Error(), "available: go, node, jdk"))
}

func appStringPtr(value string) *string {
	return &value
}
