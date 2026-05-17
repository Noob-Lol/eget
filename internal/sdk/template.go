package sdk

import (
	"fmt"
	"strings"
)

type TemplateVars struct {
	Name    string
	Version string
	OS      string
	Arch    string
	Ext     string
}

func RenderTemplate(pattern string, vars TemplateVars) (string, error) {
	replacer := strings.NewReplacer(
		"{name}", vars.Name,
		"{version}", vars.Version,
		"{os}", vars.OS,
		"{arch}", vars.Arch,
		"{ext}", vars.Ext,
	)
	rendered := replacer.Replace(pattern)
	if strings.Contains(rendered, "{") || strings.Contains(rendered, "}") {
		return "", fmt.Errorf("unknown template variable in %q", pattern)
	}
	return rendered, nil
}
