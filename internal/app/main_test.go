package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("EGET_CONFIG") == "" {
		dir, err := os.MkdirTemp("", "eget-app-tests-*")
		if err != nil {
			panic(err)
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "eget.toml")
		if err := os.WriteFile(path, []byte(""), 0644); err != nil {
			panic(err)
		}
		_ = os.Setenv("EGET_CONFIG", path)
	}
	os.Exit(m.Run())
}
