package sdk

import (
	"fmt"
	"regexp"
	"strings"
)

var versionPrefixPattern = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

func ParseTarget(input string) (Target, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Target{}, fmt.Errorf("sdk target is empty")
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return Target{}, fmt.Errorf("invalid sdk target %q: spaces are not supported", input)
	}

	at := strings.Count(raw, "@")
	colon := strings.Count(raw, ":")
	if at+colon > 1 {
		return Target{}, fmt.Errorf("invalid sdk target %q", input)
	}

	name := raw
	version := "latest"
	if at+colon == 1 {
		sep := "@"
		if colon == 1 {
			sep = ":"
		}
		parts := strings.SplitN(raw, sep, 2)
		name = parts[0]
		version = parts[1]
	}
	if name == "" || version == "" {
		return Target{}, fmt.Errorf("invalid sdk target %q", input)
	}

	kind := VersionExact
	if version == "latest" {
		kind = VersionLatest
	} else if versionPrefixPattern.MatchString(version) {
		kind = VersionPrefix
	}

	return Target{
		Raw:     raw,
		Name:    name,
		Version: version,
		Kind:    kind,
	}, nil
}
