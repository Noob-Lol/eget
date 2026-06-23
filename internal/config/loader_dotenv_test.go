package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestLoadDotenvFromConfigDir(t *testing.T) {
	tmp := t.TempDir()
	xdgHome := filepath.Join(tmp, "xdg")
	dotenvPath := filepath.Join(xdgHome, "eget", ".env")
	writeTestFile(t, dotenvPath, "EGET_SELF_UPDATE_SOURCE=https://example.com/tools/eget/\n")
	t.Setenv("EGET_SELF_UPDATE_SOURCE", "")

	err := LoadDotenvWithOptions(pathOptions{
		HomeDir: tmp,
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			if key == "XDG_CONFIG_HOME" {
				return xdgHome, true
			}
			return "", false
		},
	})

	assert.NoErr(t, err)
	assert.Eq(t, "https://example.com/tools/eget/", os.Getenv("EGET_SELF_UPDATE_SOURCE"))
}

func TestLoadDotenvInjectsEnvForConfigParseEnv(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	dotenvPath := filepath.Join(tmp, "xdg", "eget", ".env")
	writeTestFile(t, dotenvPath, "GITHUB_TOKEN=secret-token\nPROXY_URL=http://127.0.0.1:7890\n")
	writeTestFile(t, configPath, `
[global]
github_token = "${GITHUB_TOKEN}"
proxy_url = "${PROXY_URL}"
`)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PROXY_URL", "")

	err := LoadDotenvWithOptions(pathOptions{
		HomeDir: tmp,
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			if key == "XDG_CONFIG_HOME" {
				return filepath.Join(tmp, "xdg"), true
			}
			return "", false
		},
	})
	assert.NoErr(t, err)

	cfg, err := LoadFile(configPath)
	assert.NoErr(t, err)
	assert.Eq(t, "secret-token", *cfg.Global.GithubToken)
	assert.Eq(t, "http://127.0.0.1:7890", *cfg.Global.ProxyURL)
}

func TestLoadHTTPProxyURLEnvExpansion(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	writeTestFile(t, configPath, `
[http_proxy]
url = "${PROXY_URL}"
`)
	t.Setenv("PROXY_URL", "http://127.0.0.1:10801")

	cfg, err := LoadFile(configPath)

	assert.NoErr(t, err)
	assert.Eq(t, "http://127.0.0.1:10801", *cfg.HTTPProxy.URL)
}
