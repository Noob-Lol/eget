package urltemplate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type Config struct {
	URLTemplate         string
	LatestURL           string
	LatestFormat        string
	LatestJSONPath      string
	VersionRegex        string
	OSMap               map[string]string
	ArchMap             map[string]string
	ExtMap              map[string]string
	LibcMap             map[string]string
	ChecksumURLTemplate string
	ChecksumFormat      string
	ChecksumJSONPath    string
	ChecksumRegex       string
	InstallAction       string
	InstallArgs         []string
}

type VariableInput struct {
	Name    string
	Version string
	GOOS    string
	GOARCH  string
	Libc    string
	Config  Config
}

func VariablesFor(input VariableInput) (map[string]string, error) {
	goos := mappedValue(input.Config.OSMap, input.GOOS)
	goarch := mappedValue(input.Config.ArchMap, input.GOARCH)
	libc := ""
	if input.GOOS == "linux" && input.Libc != "" {
		libc = mappedValue(input.Config.LibcMap, input.Libc)
	}

	return map[string]string{
		"name":    input.Name,
		"version": input.Version,
		"os":      goos,
		"arch":    goarch,
		"ext":     mappedValue(input.Config.ExtMap, input.GOOS),
		"libc":    libc,
	}, nil
}

func Render(template string, vars map[string]string) (string, error) {
	var missing string
	rendered := regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`).ReplaceAllStringFunc(template, func(match string) string {
		key := strings.Trim(match, "{}")
		value, ok := vars[key]
		if !ok {
			missing = key
			return match
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("unknown template variable %q", missing)
	}
	return rendered, nil
}

func ParseLatest(data []byte, cfg Config) (string, error) {
	value, err := parseMetadata(data, cfg.LatestFormat, cfg.LatestJSONPath, cfg.VersionRegex, "latest")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func ParseChecksum(data []byte, cfg Config) (string, error) {
	value, err := parseMetadata(data, cfg.ChecksumFormat, cfg.ChecksumJSONPath, cfg.ChecksumRegex, "checksum")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func ExtractJSONPath(data []byte, path string) (string, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return "", err
	}
	current := root
	for _, part := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("json path %q not found", path)
		}
		current, ok = obj[part]
		if !ok {
			return "", fmt.Errorf("json path %q not found", path)
		}
	}
	switch value := current.(type) {
	case string:
		return value, nil
	case float64, bool:
		return fmt.Sprint(value), nil
	default:
		return "", fmt.Errorf("json path %q is not a scalar", path)
	}
}

func parseMetadata(data []byte, format, jsonPath, regex, field string) (string, error) {
	if format == "" {
		format = "text"
	}
	var value string
	var err error
	switch format {
	case "text":
		value = string(data)
	case "json":
		if jsonPath == "" {
			return "", fmt.Errorf("%s json path is required", field)
		}
		value, err = ExtractJSONPath(data, jsonPath)
	default:
		return "", fmt.Errorf("unsupported %s format %q", field, format)
	}
	if err != nil {
		return "", err
	}
	if regex != "" {
		return extractRegex(value, regex, field)
	}
	return value, nil
}

func extractRegex(value, pattern, field string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	matches := re.FindStringSubmatch(value)
	if len(matches) == 0 {
		return "", fmt.Errorf("%s regex did not match", field)
	}
	if len(matches) > 1 {
		return matches[1], nil
	}
	return matches[0], nil
}

func mappedValue(items map[string]string, key string) string {
	if items == nil {
		return key
	}
	value, ok := items[key]
	if !ok {
		return key
	}
	return value
}
