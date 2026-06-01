package util

import (
	"os"
	"path/filepath"
	"strings"
)

func BoolPtr(value bool) *bool {
	return &value
}

func StringPtr(value string) *string {
	return &value
}

func DerefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func ContainsPathSeparator(value string) bool {
	for _, ch := range value {
		if ch == os.PathSeparator || ch == '/' || ch == '\\' {
			return true
		}
	}
	return false
}

func NormalizeSlashesLower(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "\\", "/"))
}

func CloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ExpandPathOrRaw(path string) string {
	expanded, err := Expand(path)
	if err != nil {
		return path
	}
	return expanded
}

func FileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && !info.IsDir()
}

func DirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && info.IsDir()
}
