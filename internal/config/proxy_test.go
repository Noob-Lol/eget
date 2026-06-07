package config

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestResolveHTTPProxyUsesHTTPProxyURLByDefault(t *testing.T) {
	url := "http://127.0.0.1:10801"
	cfg := NewFile()
	cfg.HTTPProxy.URL = &url

	got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

	assert.True(t, got.Enabled)
	assert.Eq(t, url, got.URL)
}

func TestResolveHTTPProxyEnableFalseDisables(t *testing.T) {
	url := "http://127.0.0.1:10801"
	enabled := false
	cfg := NewFile()
	cfg.HTTPProxy.URL = &url
	cfg.HTTPProxy.Enable = &enabled

	got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

	assert.False(t, got.Enabled)
	assert.Eq(t, "", got.URL)
}

func TestResolveHTTPProxyFallsBackToLegacyGlobalProxyURL(t *testing.T) {
	legacy := "http://127.0.0.1:10801"
	cfg := NewFile()
	cfg.Global.ProxyURL = &legacy

	got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

	assert.True(t, got.Enabled)
	assert.Eq(t, legacy, got.URL)
}

func TestResolveHTTPProxyPrefersNewBlockOverLegacy(t *testing.T) {
	legacy := "http://127.0.0.1:10801"
	current := "http://127.0.0.1:10802"
	cfg := NewFile()
	cfg.Global.ProxyURL = &legacy
	cfg.HTTPProxy.URL = &current

	got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

	assert.Eq(t, current, got.URL)
}

func TestResolveHTTPProxyEmptyHTTPProxyURLDisablesLegacyFallback(t *testing.T) {
	legacy := "http://127.0.0.1:10801"
	empty := ""
	cfg := NewFile()
	cfg.Global.ProxyURL = &legacy
	cfg.HTTPProxy.URL = &empty

	got := ResolveHTTPProxy(cfg, ProxyResolveOptions{})

	assert.False(t, got.Enabled)
	assert.Eq(t, "", got.URL)
}

func TestResolveHTTPProxyNoProxyDisables(t *testing.T) {
	url := "http://127.0.0.1:10801"
	cfg := NewFile()
	cfg.HTTPProxy.URL = &url

	got := ResolveHTTPProxy(cfg, ProxyResolveOptions{NoProxy: true})

	assert.False(t, got.Enabled)
	assert.Eq(t, "", got.URL)
}

func TestResolveHTTPProxyEnvDisableValues(t *testing.T) {
	url := "http://127.0.0.1:10801"
	for _, value := range []string{"1", "true", "yes", "on"} {
		t.Run(value, func(t *testing.T) {
			cfg := NewFile()
			cfg.HTTPProxy.URL = &url

			got := ResolveHTTPProxy(cfg, ProxyResolveOptions{EnvNoProxy: value})

			assert.False(t, got.Enabled)
			assert.Eq(t, "", got.URL)
		})
	}
}

func TestResolveHTTPProxyMergesExcludeRules(t *testing.T) {
	url := "http://127.0.0.1:10801"
	cfg := NewFile()
	cfg.HTTPProxy.URL = &url
	cfg.HTTPProxy.Exclude = []string{"mydev.com"}

	got := ResolveHTTPProxy(cfg, ProxyResolveOptions{EnvNoProxy: "*.corp.local,10.0.0.0/8"})

	assert.True(t, got.Enabled)
	assert.Eq(t, []string{"mydev.com", "*.corp.local", "10.0.0.0/8"}, got.Exclude)
}

func TestResolveHTTPProxyOverridePriority(t *testing.T) {
	current := "http://127.0.0.1:10801"
	repo := "http://127.0.0.1:10802"
	pkg := "http://127.0.0.1:10803"
	override := "http://127.0.0.1:10804"
	cfg := NewFile()
	cfg.HTTPProxy.URL = &current

	cases := []struct {
		name string
		opts ProxyResolveOptions
		want string
	}{
		{"repo", ProxyResolveOptions{RepoURL: repo}, repo},
		{"package", ProxyResolveOptions{RepoURL: repo, PackageURL: pkg}, pkg},
		{"override", ProxyResolveOptions{RepoURL: repo, PackageURL: pkg, OverrideURL: override}, override},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveHTTPProxy(cfg, tt.opts)

			assert.True(t, got.Enabled)
			assert.Eq(t, tt.want, got.URL)
		})
	}
}

func TestParseNoProxyExclude(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  []string
	}{
		{"empty", "", nil},
		{"disable", "true", nil},
		{"rules", " mydev.com,*.corp.local,,*,10.0.0.0/8 ", []string{"mydev.com", "*.corp.local", "10.0.0.0/8"}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Eq(t, tt.want, ParseNoProxyExclude(tt.value))
		})
	}
}

func TestProxyExcludedMatchesRules(t *testing.T) {
	rules := []string{
		"localhost",
		"127.0.0.1",
		"mydev.com",
		"*.corp.local",
		".internal.local",
		"10.0.0.0/8",
		"port.example.com:8080",
	}

	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"mydev.com", true},
		{"api.mydev.com", true},
		{"api.corp.local", true},
		{"corp.local", false},
		{"api.internal.local", true},
		{"10.2.3.4", true},
		{"port.example.com:8080", true},
		{"port.example.com:9090", false},
		{"github.com", false},
	}

	for _, tt := range cases {
		t.Run(tt.host, func(t *testing.T) {
			assert.Eq(t, tt.want, ProxyExcluded(tt.host, rules))
		})
	}
}

func TestProxyExcludedIgnoresStarRule(t *testing.T) {
	assert.False(t, ProxyExcluded("github.com", []string{"*"}))
}
