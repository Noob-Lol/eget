package sdk

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input   string
		name    string
		version string
		kind    VersionKind
	}{
		{"go", "go", "latest", VersionLatest},
		{"go@latest", "go", "latest", VersionLatest},
		{"go:latest", "go", "latest", VersionLatest},
		{"go@1.21", "go", "1.21", VersionPrefix},
		{"go:1.21", "go", "1.21", VersionPrefix},
		{"jdk@21", "jdk", "21", VersionPrefix},
		{"go@1.21.1", "go", "1.21.1", VersionExact},
		{"go:1.21.1", "go", "1.21.1", VersionExact},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if err != nil {
				t.Fatalf("parse target: %v", err)
			}
			assert.Eq(t, tt.input, got.Raw)
			assert.Eq(t, tt.name, got.Name)
			assert.Eq(t, tt.version, got.Version)
			assert.Eq(t, tt.kind, got.Kind)
		})
	}
}

func TestParseTargetRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "go@", "go:", "@1.21.1", ":1.21.1", "go 1.21.1", "go@@1.21", "go@:1.21"} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseTarget(input)
			assert.Err(t, err)
		})
	}
}
