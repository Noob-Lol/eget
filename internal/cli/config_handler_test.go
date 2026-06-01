package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/app"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

func TestHandleConfigDoctorPrintsLocalPaths(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", "")
	configPath := filepath.Join(tmp, "eget.toml")
	cacheDir := filepath.Join(tmp, "cache")
	targetDir := filepath.Join(tmp, "bin")
	sdkDir := filepath.Join(tmp, "sdks")
	assert.NoErr(t, os.MkdirAll(cacheDir, 0o755))
	assert.NoErr(t, os.MkdirAll(targetDir, 0o755))
	assert.NoErr(t, os.MkdirAll(sdkDir, 0o755))
	assert.NoErr(t, os.WriteFile(configPath, []byte("[global]\n"), 0o644))

	cacheValue := cacheDir
	targetValue := targetDir
	sdkValue := sdkDir
	proxyValue := "http://127.0.0.1:10801"
	tokenValue := "secret"
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &cacheValue
	cfg.Global.Target = &targetValue
	cfg.Global.SDKTarget = &sdkValue
	cfg.Global.ProxyURL = &proxyValue
	cfg.Global.GithubToken = &tokenValue
	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: configPath,
			Load: func() (*cfgpkg.File, error) {
				return cfg, nil
			},
		},
		lookupEnv: func(key string) (string, bool) {
			switch key {
			case "EGET_CONFIG":
				return configPath, true
			case "EGET_CONFIG_DIR":
				return filepath.Dir(configPath), true
			case "EGET_BIN":
				return targetDir, true
			case "EGET_GITHUB_TOKEN":
				return "env-secret", true
			case "EGET_SELF_UPDATE_SOURCE":
				return "https://example.com/tools/eget/", true
			default:
				return "", false
			}
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleConfig(&ConfigOptions{Action: "doctor"})

	assert.NoErr(t, err)
	got := out.String()
	assert.Contains(t, got, "Eget config doctor result")
	assert.Contains(t, got, "[Config]")
	assert.Contains(t, got, "[Cache]")
	assert.Contains(t, got, "[Store]")
	assert.Contains(t, got, "[Runtime]")
	assert.Contains(t, got, "[Environment]")
	assert.Contains(t, got, configPath)
	assert.Contains(t, got, cacheDir)
	assert.Contains(t, got, filepath.Join(cacheDir, "pkg-cache"))
	assert.Contains(t, got, targetDir)
	assert.Contains(t, got, sdkDir)
	assert.Contains(t, got, filepath.Join(tmp, ".config", "eget", "installed.toml"))
	assert.Contains(t, got, filepath.Join(tmp, ".config", "eget", "sdk.installed.json"))
	assert.Contains(t, got, "github_token: set")
	assert.Contains(t, got, "EGET_CONFIG: "+configPath)
	assert.Contains(t, got, "EGET_CONFIG_DIR: "+filepath.Dir(configPath))
	assert.Contains(t, got, "EGET_BIN: "+targetDir)
	assert.Contains(t, got, "EGET_GITHUB_TOKEN: set")
	assert.Contains(t, got, "EGET_SELF_UPDATE_SOURCE: https://example.com/tools/eget/")
	assert.NotContains(t, got, "secret")
	assert.NotContains(t, got, "env-secret")
}

func TestHandleConfigDoctorKeepsConfigDirIndependentFromExplicitConfigFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("EGET_CONFIG_DIR", "")
	configPath := filepath.Join(tmp, "external", "eget.windows.toml")
	assert.NoErr(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	assert.NoErr(t, os.WriteFile(configPath, []byte("[global]\n"), 0o644))

	cfg := cfgpkg.NewFile()
	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: configPath,
			Load: func() (*cfgpkg.File, error) {
				return cfg, nil
			},
		},
		lookupEnv: func(key string) (string, bool) {
			if key == "EGET_CONFIG" {
				return configPath, true
			}
			return "", false
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleConfig(&ConfigOptions{Action: "doctor"})

	assert.NoErr(t, err)
	got := out.String()
	defaultConfigDir := filepath.Join(tmp, ".config", "eget")
	assert.Contains(t, got, "config_file: "+configPath)
	assert.Contains(t, got, "config_dir: "+defaultConfigDir)
	assert.Contains(t, got, "dotenv_file: "+filepath.Join(defaultConfigDir, ".env"))
	assert.NotContains(t, got, "config_dir: "+filepath.Dir(configPath))
}

func TestHandleConfigInitRejectsOverwriteWithoutConfirmation(t *testing.T) {
	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: "testdata/eget.toml",
			Load: func() (*cfgpkg.File, error) {
				cfg := cfgpkg.NewFile()
				target := "~/bin"
				cfg.Global.Target = &target
				return cfg, nil
			},
		},
	}

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	if _, err := w.WriteString("n\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	err = svc.handleConfig(&ConfigOptions{Action: "init"})
	if err == nil {
		t.Fatal("expected overwrite rejection error")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestHandleConfigInitTreatsBlankOverwriteConfirmationAsCancel(t *testing.T) {
	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: "testdata/eget.toml",
			Load: func() (*cfgpkg.File, error) {
				return cfgpkg.NewFile(), nil
			},
		},
	}

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	if _, err := w.WriteString("\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	err = svc.handleConfig(&ConfigOptions{Action: "init"})
	if err == nil {
		t.Fatal("expected blank confirmation to cancel")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestHandleConfigInitAllowsOverwriteWithConfirmation(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")

	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: configPath,
		},
	}

	if err := os.WriteFile(configPath, []byte("[global]\ntarget = \"~/bin\"\n"), 0o644); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	if _, err := w.WriteString("y\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	if err := svc.handleConfig(&ConfigOptions{Action: "init"}); err != nil {
		t.Fatalf("expected overwrite confirmation to allow init, got %v", err)
	}

	cfg, err := cfgpkg.LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Global.Target == nil || *cfg.Global.Target != "~/.local/bin" {
		t.Fatalf("expected config to be overwritten with defaults, got %#v", cfg.Global.Target)
	}
}

func TestHandleConfigPathPrintsPath(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	writeCLIFile(t, configPath, "[global]\ncache_dir = \"~/cache\"\n")
	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: configPath,
			Load: func() (*cfgpkg.File, error) {
				return cfgpkg.LoadFile(configPath)
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleConfig(&ConfigOptions{Action: "path", Target: "config_file"})
	assert.NoErr(t, err)
	assert.Eq(t, configPath+"\n", out.String())
}

func TestHandleConfigPathCheckPrintsExistsStatus(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "eget.toml")
	cacheDir := filepath.Join(tmp, "cache")
	writeCLIFile(t, configPath, "[global]\ncache_dir = \""+filepath.ToSlash(cacheDir)+"\"\n")
	assert.NoErr(t, os.MkdirAll(cacheDir, 0o755))
	svc := &cliService{
		cfgService: app.ConfigService{
			ConfigPath: configPath,
			Load: func() (*cfgpkg.File, error) {
				return cfgpkg.LoadFile(configPath)
			},
		},
	}

	var out bytes.Buffer
	ccolor.SetOutput(&out)
	defer ccolor.SetOutput(os.Stdout)

	err := svc.handleConfig(&ConfigOptions{Action: "path", Target: "cache_dir", Check: true})
	assert.NoErr(t, err)
	assert.Eq(t, filepath.ToSlash(cacheDir)+", exists: true\n", filepath.ToSlash(out.String()))
}
