package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/gookit/goutil/testutil/assert"
)

func TestParseOlderDuration(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{"minutes", "30m", 30 * time.Minute},
		{"hours", "12h", 12 * time.Hour},
		{"days", "3d", 72 * time.Hour},
		{"weeks", "1w", 7 * 24 * time.Hour},
		{"go duration", "72h", 72 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOlderDuration(tt.input)
			assert.NoErr(t, err)
			assert.Eq(t, tt.want, got)
		})
	}
}

func TestParseOlderDurationRejectsInvalidInput(t *testing.T) {
	tests := []string{"", "0", "0d", "-1d", "1mo", "abc"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseOlderDuration(input)
			assert.Err(t, err)
		})
	}
}

func TestServiceResolveCacheDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := cfgpkg.NewFile()
	cfg.Global.CacheDir = &tmp
	service := Service{Config: cfg}

	got, err := service.ResolveCacheDir()

	assert.NoErr(t, err)
	assert.Eq(t, tmp, got)
}

func TestServiceResolveCacheDirUsesDefault(t *testing.T) {
	service := Service{Config: cfgpkg.NewFile()}

	got, err := service.ResolveCacheDir()

	assert.NoErr(t, err)
	assert.Contains(t, got, ".cache")
	assert.Contains(t, got, "eget")
}

func TestServiceRejectsDangerousCacheDir(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{"empty", ""},
		{"root", filepath.VolumeName(filepath.Clean(os.TempDir())) + string(filepath.Separator)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCacheDirForMutation(tt.dir)
			assert.Err(t, err)
		})
	}
}
