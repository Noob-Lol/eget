package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gookit/goutil/envutil"
	"github.com/inherelab/eget/internal/util"
)

func LoadDotenv() error {
	homeDir, err := util.Home()
	if err != nil {
		return err
	}
	return LoadDotenvWithOptions(pathOptions{
		HomeDir:   homeDir,
		GOOS:      runtime.GOOS,
		LookupEnv: os.LookupEnv,
	})
}

func LoadDotenvWithOptions(opts pathOptions) error {
	path, err := resolveDotenvPath(opts)
	if err != nil {
		return err
	}
	restoreEmptyEnv, err := unsetEmptyDotenvKeys(path)
	if err != nil {
		return err
	}
	defer restoreEmptyEnv()
	return envutil.DotenvLoad(func(cfg *envutil.Dotenv) {
		cfg.Files = []string{path}
		cfg.IgnoreNotExist = true
	})
}

func unsetEmptyDotenvKeys(path string) (func(), error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return func() {}, nil
		}
		return nil, err
	}

	unset := make([]string, 0)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(key, "export ")))
		if key == "" {
			continue
		}
		if value, exists := os.LookupEnv(key); exists && value == "" {
			_ = os.Unsetenv(key)
			unset = append(unset, key)
		}
	}

	return func() {
		for _, key := range unset {
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, "")
			}
		}
	}, nil
}

func resolveDotenvPath(opts pathOptions) (string, error) {
	if opts.LookupEnv == nil {
		opts.LookupEnv = os.LookupEnv
	}
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.HomeDir == "" {
		homeDir, err := util.Home()
		if err != nil {
			return "", err
		}
		opts.HomeDir = homeDir
	}

	return filepath.Join(filepath.Dir(getOSConfigPath(opts)), ".env"), nil
}
