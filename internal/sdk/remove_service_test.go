package sdk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestServiceRemoveDeletesSafePathAndRecord(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	target := filepath.Join(root, "sdks", "gosdk", "go1.21.1")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	if err := store.Record(InstalledEntry{Name: "go", Version: "1.21.1", Path: target}); err != nil {
		t.Fatalf("record sdk: %v", err)
	}
	svc := Service{Config: cfg, Store: store, GOOS: "linux", GOARCH: "amd64"}

	result, err := svc.Remove("go@1.21.1")
	if err != nil {
		t.Fatalf("remove sdk: %v", err)
	}

	assert.Eq(t, target, result.Path)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected target removed, stat err=%v", err)
	}
	entries, err := store.List("go")
	if err != nil {
		t.Fatalf("list store: %v", err)
	}
	assert.Eq(t, 0, len(entries))
}

func TestServiceRemoveRejectsUnsafePath(t *testing.T) {
	root := t.TempDir()
	cfg := testSDKConfig(root)
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside: %v", err)
	}
	store := Store{Path: filepath.Join(root, "sdk.installed.json")}
	if err := store.Record(InstalledEntry{Name: "go", Version: "1.21.1", Path: outside}); err != nil {
		t.Fatalf("record sdk: %v", err)
	}
	svc := Service{Config: cfg, Store: store, GOOS: "linux", GOARCH: "amd64"}

	_, err := svc.Remove("go@1.21.1")
	assert.Err(t, err)
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("expected unsafe path to remain: %v", statErr)
	}
}
