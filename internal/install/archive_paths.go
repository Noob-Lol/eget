package install

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func archiveChildPath(parent, name string) (string, bool) {
	parent = archivePathForCompare(parent)
	name = archivePathForCompare(name)
	if name == parent {
		return "", true
	}
	prefix := strings.TrimRight(parent, "/") + "/"
	if !strings.HasPrefix(name, prefix) {
		return "", false
	}
	return strings.TrimPrefix(name, prefix), true
}

func archivePathForCompare(name string) string {
	return path.Clean(strings.ReplaceAll(name, `\`, "/"))
}

func safeArchiveRelativePath(name string) (string, error) {
	name = archivePathForCompare(name)
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || cleanName == ".." || filepath.VolumeName(cleanName) != "" {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return cleanName, nil
}

func safeArchiveOutputPath(output, name string) (string, error) {
	if output == "" {
		output = "."
	}
	name = archivePathForCompare(name)
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || cleanName == ".." || filepath.VolumeName(cleanName) != "" {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return filepath.Join(output, cleanName), nil
}

func validateArchiveLinkTarget(name string) error {
	_, err := safeArchiveRelativePath(name)
	return err
}

func safeArchiveLinkName(name string, typ FileType) (string, error) {
	if typ != TypeLink && typ != TypeSymlink || name == "" {
		return name, nil
	}
	cleanName, err := safeArchiveRelativePath(name)
	if err != nil {
		return "", err
	}
	return cleanName, nil
}
