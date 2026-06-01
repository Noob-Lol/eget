package cli

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"

	"github.com/gookit/goutil/x/ccolor"
	cfgpkg "github.com/inherelab/eget/internal/config"
)

func (s *cliService) warnIfSudoUserConfigLooksSkipped(quiet bool) {
	if quiet {
		return
	}

	lookupEnv := os.LookupEnv
	if s.lookupEnv != nil {
		lookupEnv = s.lookupEnv
	}
	if configPath, ok := lookupEnv("EGET_CONFIG"); ok && configPath != "" {
		return
	}

	sudoUser, ok := lookupEnv("SUDO_USER")
	if !ok || sudoUser == "" || sudoUser == "root" {
		return
	}

	resolveConfigPath := cfgpkg.ResolveConfigPath
	if s.configPathResolver != nil {
		resolveConfigPath = s.configPathResolver
	}
	if _, err := resolveConfigPath(); err == nil {
		return
	} else if !cfgpkg.IsNotExist(err) && !errors.Is(err, os.ErrNotExist) {
		return
	}

	lookupHome := lookupUserHome
	if s.lookupUserHome != nil {
		lookupHome = s.lookupUserHome
	}
	homeDir, err := lookupHome(sudoUser)
	if err != nil || homeDir == "" {
		return
	}

	candidate := cfgpkg.OSConfigPath(homeDir, "linux", lookupEnv)
	exists := fileExists
	if s.fileExists != nil {
		exists = s.fileExists
	}
	if !exists(candidate) {
		return
	}

	displayPath := filepath.ToSlash(candidate)
	ccolor.Fprintf(
		s.stderrWriter(),
		"<yellow>Warning</>: sudo may be using a different HOME, so eget did not load %s. Try: sudo EGET_CONFIG=%q eget install ...\n",
		displayPath,
		displayPath,
	)
}

func lookupUserHome(name string) (string, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && !info.IsDir()
}
