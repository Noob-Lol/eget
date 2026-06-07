package config

import (
	"net"
	"strconv"
	"strings"
)

type ProxyConfig struct {
	Enabled bool
	URL     string
	Exclude []string
}

type ProxyResolveOptions struct {
	NoProxy     bool
	EnvNoProxy  string
	OverrideURL string
	PackageURL  string
	RepoURL     string
}

func ResolveHTTPProxy(cfg *File, opts ProxyResolveOptions) ProxyConfig {
	if opts.NoProxy || noProxyEnvDisables(opts.EnvNoProxy) {
		return ProxyConfig{}
	}

	exclude := []string{}
	if cfg != nil {
		exclude = append(exclude, cfg.HTTPProxy.Exclude...)
	}
	exclude = append(exclude, ParseNoProxyExclude(opts.EnvNoProxy)...)

	proxyURL := firstNonEmptyProxyURL(opts.OverrideURL, opts.PackageURL, opts.RepoURL)
	if proxyURL == "" && cfg != nil {
		if httpProxyConfigured(cfg.HTTPProxy) {
			if cfg.HTTPProxy.Enable != nil && !*cfg.HTTPProxy.Enable {
				return ProxyConfig{Exclude: exclude}
			}
			proxyURL = derefString(cfg.HTTPProxy.URL)
		} else {
			proxyURL = derefString(cfg.Global.ProxyURL)
		}
	}

	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return ProxyConfig{Exclude: exclude}
	}

	return ProxyConfig{Enabled: true, URL: proxyURL, Exclude: exclude}
}

func ParseNoProxyExclude(value string) []string {
	if noProxyEnvDisables(value) {
		return nil
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "*" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ProxyExcluded(host string, rules []string) bool {
	hostName, hostPort := splitHostPort(strings.ToLower(strings.TrimSpace(host)))
	if hostName == "" {
		return false
	}

	for _, rule := range rules {
		rule = strings.ToLower(strings.TrimSpace(rule))
		if rule == "" || rule == "*" {
			continue
		}

		ruleHost, rulePort := splitHostPort(rule)
		if rulePort != "" && rulePort != hostPort {
			continue
		}
		if proxyRuleMatchesHost(hostName, ruleHost) {
			return true
		}
	}
	return false
}

func proxyRuleMatchesHost(hostName, ruleHost string) bool {
	if _, ipNet, err := net.ParseCIDR(ruleHost); err == nil {
		if ip := net.ParseIP(hostName); ip != nil && ipNet.Contains(ip) {
			return true
		}
		return false
	}
	if ruleHost == hostName {
		return true
	}
	if strings.HasPrefix(ruleHost, "*.") {
		suffix := strings.TrimPrefix(ruleHost, "*.")
		return strings.HasSuffix(hostName, "."+suffix)
	}
	if strings.HasPrefix(ruleHost, ".") {
		suffix := strings.TrimPrefix(ruleHost, ".")
		return strings.HasSuffix(hostName, "."+suffix)
	}
	return strings.HasSuffix(hostName, "."+ruleHost)
}

func splitHostPort(value string) (string, string) {
	if host, port, err := net.SplitHostPort(value); err == nil {
		return strings.Trim(host, "[]"), port
	}
	if idx := strings.LastIndexByte(value, ':'); idx > 0 && !strings.Contains(value[:idx], ":") {
		if _, err := strconv.Atoi(value[idx+1:]); err == nil {
			return value[:idx], value[idx+1:]
		}
	}
	return strings.Trim(value, "[]"), ""
}

func noProxyEnvDisables(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func httpProxyConfigured(section HTTPProxySection) bool {
	return section.Enable != nil || section.URL != nil || len(section.Exclude) > 0
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmptyProxyURL(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
