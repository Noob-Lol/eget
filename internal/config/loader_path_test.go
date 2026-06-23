package config

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestResolveConfigPathPrefersEnv(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "env.toml")
	homePath := filepath.Join(tmp, "home")

	writeTestFile(t, envPath, "title = 'env'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir: homePath,
		GOOS:    runtime.GOOS,
		LookupEnv: func(key string) (string, bool) {
			if key == "EGET_CONFIG" {
				return envPath, true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}

	if path != envPath {
		t.Fatalf("expected env path %q, got %q", envPath, path)
	}
}

func TestResolveConfigPathFallsBackToDotfile(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	dotfile := filepath.Join(homePath, ".eget.toml")

	writeTestFile(t, dotfile, "title = 'home'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir:   homePath,
		GOOS:      runtime.GOOS,
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}

	if path != dotfile {
		t.Fatalf("expected dotfile path %q, got %q", dotfile, path)
	}
}

func TestResolveConfigPathFallsBackToOSConfigDir(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")

	testCases := []struct {
		name     string
		goos     string
		envKey   string
		envValue string
		wantPath string
	}{
		{
			name:     "xdg",
			goos:     "linux",
			envKey:   "XDG_CONFIG_HOME",
			envValue: filepath.Join(tmp, "xdg"),
			wantPath: filepath.Join(tmp, "xdg", "eget", "eget.toml"),
		},
		{
			name:     "windows uses xdg env when set",
			goos:     "windows",
			envKey:   "XDG_CONFIG_HOME",
			envValue: filepath.Join(tmp, "xdg-win"),
			wantPath: filepath.Join(tmp, "xdg-win", "eget", "eget.toml"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			writeTestFile(t, tc.wantPath, "title = 'os'\n")

			path, err := resolveConfigPath(pathOptions{
				HomeDir: homePath,
				GOOS:    tc.goos,
				LookupEnv: func(key string) (string, bool) {
					if key == tc.envKey {
						return tc.envValue, true
					}
					return "", false
				},
			})
			if err != nil {
				t.Fatalf("resolve path: %v", err)
			}

			if path != tc.wantPath {
				t.Fatalf("expected os config path %q, got %q", tc.wantPath, path)
			}
		})
	}
}

func TestResolveConfigPathUsesEgetConfigDir(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "custom-config")
	wantPath := filepath.Join(configDir, "eget.toml")
	writeTestFile(t, wantPath, "title = 'custom'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir: filepath.Join(tmp, "home"),
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			if key == "EGET_CONFIG_DIR" {
				return configDir, true
			}
			return "", false
		},
	})
	assert.NoErr(t, err)
	assert.Eq(t, wantPath, path)
}

func TestResolveConfigPathPrefersEgetConfigDirOverLegacyDotfile(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	configDir := filepath.Join(tmp, "custom-config")
	wantPath := filepath.Join(configDir, "eget.toml")
	writeTestFile(t, filepath.Join(homePath, ".eget.toml"), "title = 'legacy'\n")
	writeTestFile(t, wantPath, "title = 'custom'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir: homePath,
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			if key == "EGET_CONFIG_DIR" {
				return configDir, true
			}
			return "", false
		},
	})
	assert.NoErr(t, err)
	assert.Eq(t, wantPath, path)
}

func TestResolveConfigPathPrefersEgetConfigOverEgetConfigDir(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "custom-config")
	configPath := filepath.Join(tmp, "explicit.toml")
	writeTestFile(t, filepath.Join(configDir, "eget.toml"), "title = 'custom'\n")
	writeTestFile(t, configPath, "title = 'explicit'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir: filepath.Join(tmp, "home"),
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			switch key {
			case "EGET_CONFIG":
				return configPath, true
			case "EGET_CONFIG_DIR":
				return configDir, true
			default:
				return "", false
			}
		},
	})
	assert.NoErr(t, err)
	assert.Eq(t, configPath, path)
}

func TestResolveConfigPathSkipsDotfileWhenEnvPathMissing(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	dotfile := filepath.Join(homePath, ".eget.toml")
	fallbackPath := filepath.Join(tmp, "xdg", "eget", "eget.toml")

	writeTestFile(t, dotfile, "title = 'home'\n")
	writeTestFile(t, fallbackPath, "title = 'fallback'\n")

	path, err := resolveConfigPath(pathOptions{
		HomeDir: homePath,
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			switch key {
			case "EGET_CONFIG":
				return filepath.Join(tmp, "missing.toml"), true
			case "XDG_CONFIG_HOME":
				return filepath.Join(tmp, "xdg"), true
			default:
				return "", false
			}
		},
	})
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}

	if path != fallbackPath {
		t.Fatalf("expected fallback path %q when env config is missing, got %q", fallbackPath, path)
	}
}

func TestResolveWritablePathDefaultsToOSConfigDir(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	xdgHome := filepath.Join(tmp, "xdg")
	wantPath := filepath.Join(xdgHome, "eget", "eget.toml")

	path, err := resolveWritablePath(pathOptions{
		HomeDir: homePath,
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			if key == "XDG_CONFIG_HOME" {
				return xdgHome, true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("resolve writable path: %v", err)
	}

	if path != wantPath {
		t.Fatalf("expected writable path %q, got %q", wantPath, path)
	}
}

func TestResolveWritablePathUsesEgetConfigDir(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "custom-config")
	wantPath := filepath.Join(configDir, "eget.toml")

	path, err := resolveWritablePath(pathOptions{
		HomeDir: filepath.Join(tmp, "home"),
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			if key == "EGET_CONFIG_DIR" {
				return configDir, true
			}
			return "", false
		},
	})
	assert.NoErr(t, err)
	assert.Eq(t, wantPath, path)
}

func TestResolveWritablePathDefaultsToHomeConfigDirOnWindows(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")
	wantPath := filepath.Join(homePath, ".config", "eget", "eget.toml")

	path, err := resolveWritablePath(pathOptions{
		HomeDir:   homePath,
		GOOS:      "windows",
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("resolve writable path: %v", err)
	}

	if path != wantPath {
		t.Fatalf("expected writable path %q, got %q", wantPath, path)
	}
}

func TestResolveDotenvPathUsesOSConfigDir(t *testing.T) {
	tmp := t.TempDir()
	homePath := filepath.Join(tmp, "home")

	path, err := resolveDotenvPath(pathOptions{
		HomeDir: homePath,
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			if key == "XDG_CONFIG_HOME" {
				return filepath.Join(tmp, "xdg"), true
			}
			return "", false
		},
	})

	assert.NoErr(t, err)
	assert.Eq(t, filepath.Join(tmp, "xdg", "eget", ".env"), path)
}

func TestResolveDotenvPathUsesEgetConfigDirEvenWithExplicitConfig(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "custom-config")

	path, err := resolveDotenvPath(pathOptions{
		HomeDir: filepath.Join(tmp, "home"),
		GOOS:    "linux",
		LookupEnv: func(key string) (string, bool) {
			switch key {
			case "EGET_CONFIG":
				return filepath.Join(tmp, "explicit.toml"), true
			case "EGET_CONFIG_DIR":
				return configDir, true
			default:
				return "", false
			}
		},
	})

	assert.NoErr(t, err)
	assert.Eq(t, filepath.Join(configDir, ".env"), path)
}
