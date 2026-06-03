package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

type commandCall struct {
	name    string
	options any
}

func TestMain_NoSubcommandReturnsErrorAndHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Main([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error for missing subcommand, got %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatalf("expected help output on stdout")
	}
	help := stdout.String()
	if !strings.Contains(help, "Usage") && !strings.Contains(help, "Commands") {
		t.Fatalf("expected help output to contain usage, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected stderr to be empty, got %q", stderr.String())
	}
}

func TestSetBuildInfoCompactsBuildTime(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"rfc3339 offset", "2026-05-05T13:20:19+08:00", "2026-05-05T13:20:19"},
		{"makefile legacy", "2026/05/05-13:20:19", "2026-05-05T13:20:19"},
		{"unknown", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetBuildInfo("dev", "hash", tt.input)
			assert.Eq(t, tt.want, buildTime)
		})
	}
}

func TestBuildInfoReturnsConfiguredValues(t *testing.T) {
	SetBuildInfo("1.7.1", "abcdef12", "2026-05-25T10:20:30+08:00")

	info := BuildInfo()

	assert.Eq(t, "1.7.1", info.Version)
	assert.Eq(t, "abcdef12", info.GitHash)
	assert.Eq(t, "2026-05-25T10:20:30", info.BuildTime)
}

func TestMain_VersionUsesBuildInfo(t *testing.T) {
	SetBuildInfo("v1.2.3", "abc123", "2026-05-16T10:11:12+08:00")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := newApp(func(string, any) error {
		t.Fatalf("handler should not run for version")
		return nil
	}, &stdout, &stderr).RunWithArgs([]string{"--version"})

	assert.NoErr(t, err)
	out := stdout.String() + stderr.String()
	assert.Contains(t, out, "v1.2.3")
}

func TestMain_GlobalVerboseFlagParsesBeforeCommand(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newApp(handler, &stdout, &stderr)
	err := app.RunWithArgs([]string{"-v", "install", "inhere/markview"})
	if err != nil {
		t.Fatalf("expected verbose install command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}
	if !app.Verbose() {
		t.Fatalf("expected app verbose flag to be true")
	}
}

func TestMain_GlobalNoProxyFlagParsesBeforeCommand(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newApp(handler, &stdout, &stderr)
	err := app.RunWithArgs([]string{"--no-proxy", "install", "inhere/markview"})
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	assert.True(t, app.NoProxy())
}
