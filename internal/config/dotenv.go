package config

import (
	"os"
	"path/filepath"
	"runtime"

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
	return envutil.DotenvLoad(func(cfg *envutil.Dotenv) {
		cfg.Files = []string{path}
		cfg.IgnoreNotExist = true
	})
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
