package urltemplate

import (
	"fmt"
	"strings"
)

const Prefix = "template:"

type Target struct {
	ID         string
	Normalized string
}

func IsTarget(value string) bool {
	return strings.HasPrefix(value, Prefix)
}

func ParseTarget(value string) (Target, error) {
	if !IsTarget(value) {
		return Target{}, fmt.Errorf("invalid template target %q", value)
	}
	id := strings.TrimSpace(strings.TrimPrefix(value, Prefix))
	if id == "" {
		return Target{}, fmt.Errorf("invalid template target %q", value)
	}
	return Target{ID: id, Normalized: Prefix + id}, nil
}
