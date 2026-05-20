package urltemplate

import (
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantID  string
		wantErr string
	}{
		{name: "valid", input: "template:claude", wantID: "claude"},
		{name: "empty id", input: "template:", wantErr: "invalid template target"},
		{name: "not template", input: "owner/repo", wantErr: "invalid template target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			assert.NoErr(t, err)
			assert.Eq(t, tt.wantID, got.ID)
			assert.Eq(t, tt.input, got.Normalized)
		})
	}
}
