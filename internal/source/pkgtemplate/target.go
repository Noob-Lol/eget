package pkgtemplate

import (
	"fmt"
	"strings"
)

const Prefix = "pkg-template:"

type Target struct {
	Template   string
	Package    string
	Normalized string
}

func IsTarget(value string) bool {
	return strings.HasPrefix(value, Prefix)
}

func ParseTarget(value string) (Target, error) {
	if !IsTarget(value) {
		return Target{}, fmt.Errorf("invalid pkg-template target %q", value)
	}
	rest := strings.TrimPrefix(value, Prefix)
	parts := strings.Split(rest, ":")
	if len(parts) != 2 {
		return Target{}, fmt.Errorf("invalid pkg-template target %q", value)
	}
	template := strings.TrimSpace(parts[0])
	pkg := strings.TrimSpace(parts[1])
	if template == "" || pkg == "" {
		return Target{}, fmt.Errorf("invalid pkg-template target %q", value)
	}
	return Target{
		Template:   template,
		Package:    pkg,
		Normalized: Prefix + template + ":" + pkg,
	}, nil
}

func ResolveAlias(value string, templates map[string]struct{}) (string, bool) {
	if IsTarget(value) {
		target, err := ParseTarget(value)
		if err != nil {
			return "", false
		}
		return target.Normalized, true
	}
	prefix, name, ok := strings.Cut(value, ":")
	if !ok || prefix == "" || name == "" {
		return "", false
	}
	if isKnownPrefix(prefix) {
		return "", false
	}
	if _, ok := templates[prefix]; !ok {
		return "", false
	}
	return Prefix + prefix + ":" + name, true
}

func isKnownPrefix(prefix string) bool {
	switch prefix {
	case "sourceforge", "gitlab", "gitea", "forgejo", "template", "http", "https", "file":
		return true
	default:
		return false
	}
}
