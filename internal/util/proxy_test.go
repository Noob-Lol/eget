package util

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestGlobalProxyDisabledKeepsLegacyNoProxyEnvBehavior(t *testing.T) {
	t.Setenv("NO_PROXY", "github.com")

	assert.True(t, GlobalProxyDisabled(false))
}

func TestGlobalProxyDisabledWithEnvUsesBooleanDisableValues(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"true", "true", true},
		{"host-list", "github.com", false},
		{"star", "*", false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Eq(t, tt.want, GlobalProxyDisabledWithEnv(false, tt.value))
		})
	}
}
