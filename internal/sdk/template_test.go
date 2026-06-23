package sdk

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestRenderTemplate(t *testing.T) {
	vars := TemplateVars{
		Name:    "go",
		Version: "1.21.1",
		OS:      "linux",
		Arch:    "amd64",
		Ext:     "tar.gz",
	}

	got, err := RenderTemplate("https://example.com/{name}{version}.{os}-{arch}.{ext}", vars)
	if err != nil {
		t.Fatalf("render template: %v", err)
	}
	assert.Eq(t, "https://example.com/go1.21.1.linux-amd64.tar.gz", got)
}

func TestRenderTemplateRejectsUnknownVariable(t *testing.T) {
	_, err := RenderTemplate("https://example.com/{version}-{unknown}.zip", TemplateVars{Version: "1.0.0"})
	assert.Err(t, err)
}
