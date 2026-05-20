package urltemplate

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestEffectiveSystemUsesExplicitSystem(t *testing.T) {
	goos, goarch, libc := EffectiveSystem("linux/amd64", "darwin", "arm64", func() string { return "musl" }, func(string, string) (string, string) {
		return "darwin", "arm64"
	})
	assert.Eq(t, "linux", goos)
	assert.Eq(t, "amd64", goarch)
	assert.Eq(t, "musl", libc)
}

func TestEffectiveSystemAppliesRosettaFixOnlyForImplicitSystem(t *testing.T) {
	goos, goarch, libc := EffectiveSystem("", "darwin", "amd64", func() string { return "" }, func(string, string) (string, string) {
		return "darwin", "arm64"
	})
	assert.Eq(t, "darwin", goos)
	assert.Eq(t, "arm64", goarch)
	assert.Eq(t, "", libc)
}
