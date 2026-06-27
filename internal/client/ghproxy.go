package client

import (
	"net/url"
	"strings"
)

func requestAttemptURLs(rawURL string, parsed *url.URL, opts Options) []string {
	if !opts.GhproxyEnabled {
		return []string{rawURL}
	}
	if parsed == nil {
		return []string{rawURL}
	}
	if !isGitHubDownloadRequest(parsed) {
		return []string{rawURL}
	}

	hosts := make([]string, 0, 1+len(opts.GhproxyFallbacks))
	if opts.GhproxyHostURL != "" {
		hosts = append(hosts, opts.GhproxyHostURL)
	}
	hosts = append(hosts, opts.GhproxyFallbacks...)
	if len(hosts) == 0 {
		return []string{rawURL}
	}

	attempts := make([]string, 0, len(hosts))
	seen := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		host = strings.TrimRight(strings.TrimSpace(host), "/")
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		attempts = append(attempts, host+"/"+rawURL)
	}
	if len(attempts) == 0 {
		return []string{rawURL}
	}
	return attempts
}
