package sdk

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	cfgpkg "github.com/inherelab/eget/internal/config"
)

func testSDKConfig(root string) *cfgpkg.File {
	cfg := cfgpkg.NewFile()
	cfg.Global.SDKTarget = stringPtr(filepath.Join(root, "sdks"))
	cfg.Global.SDKExtMap = map[string]string{"linux": "zip", "windows": "zip", "darwin": "tar.gz"}
	cfg.SDK["go"] = cfgpkg.SDKSection{
		Target:          stringPtr("gosdk/go{version}"),
		URLTemplate:     stringPtr("https://example.com/go{version}.{os}-{arch}.{ext}"),
		IndexURL:        stringPtr("https://example.com/golang/"),
		IndexFormat:     stringPtr("html"),
		FilenamePattern: stringPtr("go{version}.{os}-{arch}.{ext}"),
		StripComponents: intPtr(1),
	}
	return cfg
}

func writeSDKZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.WriteFile(path, sdkZipBytes(t, files), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}

func sdkZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func stringLen(data []byte) string {
	return intString(len(data))
}

func intString(value int) string {
	return strconv.Itoa(value)
}
