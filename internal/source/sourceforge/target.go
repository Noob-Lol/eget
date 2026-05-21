package sourceforge

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	Prefix      = "sourceforge:"
	AliasPrefix = "sf:"
)

type Target struct {
	Project    string
	Path       string
	Normalized string
}

func IsTarget(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, Prefix) || strings.HasPrefix(value, AliasPrefix) {
		return true
	}
	_, ok := parseProjectURL(value)
	return ok
}

func ParseTarget(value string) (Target, error) {
	value = strings.TrimSpace(value)
	if target, ok := parseProjectURL(value); ok {
		return target, nil
	}
	if !strings.HasPrefix(value, Prefix) && !strings.HasPrefix(value, AliasPrefix) {
		return Target{}, fmt.Errorf("invalid SourceForge target %q", value)
	}
	rest := strings.TrimPrefix(value, targetPrefix(value))
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return Target{}, fmt.Errorf("sourceforge project is required")
	}
	project, sourcePath, _ := strings.Cut(rest, "/")
	if project == "" {
		return Target{}, fmt.Errorf("sourceforge project is required")
	}
	sourcePath = strings.Trim(sourcePath, "/")
	return Target{Project: project, Path: sourcePath, Normalized: Prefix + project}, nil
}

func parseProjectURL(value string) (Target, bool) {
	if strings.HasPrefix(value, "sourceforge.net/") || strings.HasPrefix(value, "www.sourceforge.net/") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return Target{}, false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "www.sourceforge.net" {
		host = "sourceforge.net"
	}
	if host != "sourceforge.net" {
		return Target{}, false
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "projects" || parts[1] == "" {
		return Target{}, false
	}
	project, err := url.PathUnescape(parts[1])
	if err != nil || project == "" {
		return Target{}, false
	}

	sourcePath := ""
	if len(parts) > 2 && parts[2] == "files" {
		rest := parts[3:]
		for len(rest) > 0 && rest[len(rest)-1] == "download" {
			rest = rest[:len(rest)-1]
		}
		for i, part := range rest {
			unescaped, err := url.PathUnescape(part)
			if err != nil {
				return Target{}, false
			}
			rest[i] = unescaped
		}
		sourcePath = strings.Trim(strings.Join(rest, "/"), "/")
	}
	return Target{Project: project, Path: sourcePath, Normalized: Prefix + project}, true
}

func targetPrefix(value string) string {
	if strings.HasPrefix(value, AliasPrefix) {
		return AliasPrefix
	}
	return Prefix
}
