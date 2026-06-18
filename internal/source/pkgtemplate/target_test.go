package pkgtemplate

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantTemplate string
		wantPackage  string
		wantErr      string
	}{
		{name: "valid", input: "pkg-template:mydev:markview", wantTemplate: "mydev", wantPackage: "markview"},
		{name: "empty template", input: "pkg-template::markview", wantErr: "invalid pkg-template target"},
		{name: "empty package", input: "pkg-template:mydev:", wantErr: "invalid pkg-template target"},
		{name: "too many parts", input: "pkg-template:mydev:markview:extra", wantErr: "invalid pkg-template target"},
		{name: "wrong prefix", input: "template:markview", wantErr: "invalid pkg-template target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.input)
			if tt.wantErr != "" {
				assert.Err(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoErr(t, err)
			assert.Eq(t, tt.wantTemplate, got.Template)
			assert.Eq(t, tt.wantPackage, got.Package)
			assert.Eq(t, tt.input, got.Normalized)
		})
	}
}

func TestResolveAlias(t *testing.T) {
	templates := map[string]struct{}{"mydev": {}}
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "configured alias", input: "mydev:markview", want: "pkg-template:mydev:markview", wantOK: true},
		{name: "canonical", input: "pkg-template:mydev:markview", want: "pkg-template:mydev:markview", wantOK: true},
		{name: "unknown alias", input: "other:markview", wantOK: false},
		{name: "known provider sourceforge", input: "sourceforge:winmerge/stable", wantOK: false},
		{name: "known provider gitlab", input: "gitlab:owner/repo", wantOK: false},
		{name: "known provider template", input: "template:markview", wantOK: false},
		{name: "repo target", input: "owner/repo", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveAlias(tt.input, templates)
			assert.Eq(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Eq(t, tt.want, got)
			}
		})
	}
}
