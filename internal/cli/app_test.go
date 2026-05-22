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

func TestMain_InstallStandardOrderRoutesAndBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"install", "--tag", "nightly", "--rename", "tool-linux-amd64=tool", "inhere/markview"})
	if err != nil {
		t.Fatalf("expected install command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}
	if calls[0].name != "install" {
		t.Fatalf("expected command install, got %q", calls[0].name)
	}

	opts, ok := calls[0].options.(*InstallOptions)
	if !ok {
		t.Fatalf("expected InstallOptions, got %T", calls[0].options)
	}
	if opts.Tag != "nightly" {
		t.Fatalf("expected tag nightly, got %q", opts.Tag)
	}
	if opts.Rename != "tool-linux-amd64=tool" {
		t.Fatalf("expected rename option to bind, got %q", opts.Rename)
	}
	if len(opts.Targets) != 1 || opts.Targets[0] != "inhere/markview" {
		t.Fatalf("expected target inhere/markview, got %#v", opts.Targets)
	}
}

func TestMain_InstallBindsMultipleTargets(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"space separated", []string{"install", "fzf", "rg"}, []string{"fzf", "rg"}},
		{"comma separated", []string{"install", "fzf,rg,fd"}, []string{"fzf", "rg", "fd"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			assert.NoErr(t, err)
			assert.Eq(t, 1, len(calls))
			opts, ok := calls[0].options.(*InstallOptions)
			assert.True(t, ok)
			assert.Eq(t, tt.want, opts.Targets)
		})
	}
}

func TestMain_UpdateBindsMultipleTargets(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"space separated", []string{"update", "fzf", "rg"}, []string{"fzf", "rg"}},
		{"comma separated", []string{"update", "fzf,rg,fd"}, []string{"fzf", "rg", "fd"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			assert.NoErr(t, err)
			assert.Eq(t, 1, len(calls))
			opts, ok := calls[0].options.(*UpdateOptions)
			assert.True(t, ok)
			assert.Eq(t, tt.want, opts.Targets)
		})
	}
}

func TestMain_ExtractAllFlagBindsInstallDownloadAndAdd(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"install long", []string{"install", "--extract-all", "inhere/markview"}, "install"},
		{"install short", []string{"install", "--ea", "inhere/markview"}, "install"},
		{"download long", []string{"download", "--extract-all", "inhere/markview"}, "download"},
		{"download short", []string{"download", "--ea", "inhere/markview"}, "download"},
		{"add long", []string{"add", "--extract-all", "inhere/markview"}, "add"},
		{"add short", []string{"add", "--ea", "inhere/markview"}, "add"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			if err != nil {
				t.Fatalf("expected %s command to parse, got %v", tt.name, err)
			}
			if len(calls) != 1 || calls[0].name != tt.want {
				t.Fatalf("unexpected routed call: %#v", calls)
			}
			switch opts := calls[0].options.(type) {
			case *InstallOptions:
				if !opts.All {
					t.Fatalf("expected install extract-all flag to be true")
				}
			case *DownloadOptions:
				if !opts.All {
					t.Fatalf("expected download extract-all flag to be true")
				}
			case *AddOptions:
				if !opts.All {
					t.Fatalf("expected add extract-all flag to be true")
				}
			default:
				t.Fatalf("unexpected options type %T", calls[0].options)
			}
		})
	}
}

func TestMain_SDKConfigAddRoutesAndBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"sdk", "config", "add", "--mirror", "--force", "java"})
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	assert.Eq(t, "sdk.config.add", calls[0].name)
	opts, ok := calls[0].options.(*SDKConfigOptions)
	assert.True(t, ok)
	assert.Eq(t, "add", opts.Action)
	assert.Eq(t, "java", opts.Name)
	assert.True(t, opts.Mirror)
	assert.True(t, opts.Force)
	assert.False(t, opts.All)
}

func TestMain_SDKConfigAddAllRoutesAndBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"sdk", "config", "add", "--all", "--mirror"})
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	opts, ok := calls[0].options.(*SDKConfigOptions)
	assert.True(t, ok)
	assert.True(t, opts.All)
	assert.True(t, opts.Mirror)
	assert.Eq(t, "", opts.Name)
}

func TestMain_SDKConfigAddRejectsNameAndAll(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(func(string, any) error {
		t.Fatal("handler should not run")
		return nil
	}, &stdout, &stderr).RunWithArgs([]string{"sdk", "config", "add", "--all", "jdk"})

	assert.Err(t, err)
	assert.Contains(t, err.Error(), "requires exactly one")
}

func TestMain_StripComponentsFlagBindsInstallDownloadAndAdd(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"install", []string{"install", "--extract-all", "--strip-components", "1", "inhere/markview"}, "install"},
		{"download", []string{"download", "--extract-all", "--strip-components", "1", "inhere/markview"}, "download"},
		{"add", []string{"add", "--extract-all", "--strip-components", "1", "inhere/markview"}, "add"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			assert.NoErr(t, err)
			assert.Eq(t, 1, len(calls))
			assert.Eq(t, tt.want, calls[0].name)
			switch opts := calls[0].options.(type) {
			case *InstallOptions:
				assert.Eq(t, 1, opts.StripComponents)
			case *DownloadOptions:
				assert.Eq(t, 1, opts.StripComponents)
			case *AddOptions:
				assert.Eq(t, 1, opts.StripComponents)
			default:
				t.Fatalf("unexpected options type %T", calls[0].options)
			}
		})
	}
}

func TestMain_GUIFlagBindsInstallAndAdd(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"install gui", []string{"install", "--gui", "inhere/markview"}, "install"},
		{"add gui", []string{"add", "--gui", "inhere/markview"}, "add"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			if err != nil {
				t.Fatalf("expected %s command to parse, got %v", tt.name, err)
			}
			if len(calls) != 1 || calls[0].name != tt.want {
				t.Fatalf("unexpected routed call: %#v", calls)
			}
			switch opts := calls[0].options.(type) {
			case *InstallOptions:
				if !opts.GUI {
					t.Fatalf("expected install gui flag to be true")
				}
			case *AddOptions:
				if !opts.GUI {
					t.Fatalf("expected add gui flag to be true")
				}
			default:
				t.Fatalf("unexpected options type %T", calls[0].options)
			}
		})
	}
}

func TestMain_FallbackVersionsFlagBindsInstallAndDownload(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"install", []string{"install", "--fallback-versions", "10", "sourceforge:keepass/Translations 2.x"}, "install"},
		{"download", []string{"download", "--fallback-versions", "10", "sourceforge:keepass/Translations 2.x"}, "download"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			if err != nil {
				t.Fatalf("expected %s command to parse, got %v", tt.name, err)
			}
			if len(calls) != 1 || calls[0].name != tt.want {
				t.Fatalf("unexpected routed call: %#v", calls)
			}
			switch opts := calls[0].options.(type) {
			case *InstallOptions:
				if opts.FallbackVersions != 10 {
					t.Fatalf("expected install fallback versions 10, got %d", opts.FallbackVersions)
				}
			case *DownloadOptions:
				if opts.FallbackVersions != 10 {
					t.Fatalf("expected download fallback versions 10, got %d", opts.FallbackVersions)
				}
			default:
				t.Fatalf("unexpected options type %T", calls[0].options)
			}
		})
	}
}

func TestMain_ConcurrencyFlagsBindInstallDownloadUpdateAndAdd(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"install chunk", []string{"install", "--chunk", "8", "owner/repo"}, "install"},
		{"download chunk", []string{"download", "--chunk", "8", "owner/repo"}, "download"},
		{"update chunk", []string{"update", "--chunk", "8", "fd"}, "update"},
		{"add chunk", []string{"add", "--chunk", "3", "sharkdp/fd"}, "add"},
		{"install all batch", []string{"install", "--all", "--batch", "3"}, "install"},
		{"update all batch", []string{"update", "--all", "--batch", "3"}, "update"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			if err != nil {
				t.Fatalf("expected %s command to parse, got %v", tt.name, err)
			}
			if len(calls) != 1 || calls[0].name != tt.want {
				t.Fatalf("unexpected routed call: %#v", calls)
			}
			switch opts := calls[0].options.(type) {
			case *InstallOptions:
				if strings.Contains(tt.name, "chunk") && opts.ChunkConcurrency != 8 {
					t.Fatalf("expected install chunk=8, got %d", opts.ChunkConcurrency)
				}
				if strings.Contains(tt.name, "batch") && opts.BatchConcurrency != 3 {
					t.Fatalf("expected install batch=3, got %d", opts.BatchConcurrency)
				}
			case *DownloadOptions:
				if opts.ChunkConcurrency != 8 {
					t.Fatalf("expected download chunk=8, got %d", opts.ChunkConcurrency)
				}
			case *UpdateOptions:
				if strings.Contains(tt.name, "chunk") && opts.ChunkConcurrency != 8 {
					t.Fatalf("expected update chunk=8, got %d", opts.ChunkConcurrency)
				}
				if strings.Contains(tt.name, "batch") && opts.BatchConcurrency != 3 {
					t.Fatalf("expected update batch=3, got %d", opts.BatchConcurrency)
				}
			case *AddOptions:
				if opts.ChunkConcurrency != 3 {
					t.Fatalf("expected add chunk=3, got %d", opts.ChunkConcurrency)
				}
			default:
				t.Fatalf("unexpected options type %T", calls[0].options)
			}
		})
	}
}

func TestMain_DownloadRejectsGUIFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(func(string, any) error { return nil }, &stdout, &stderr).RunWithArgs([]string{"download", "--gui", "inhere/markview"})
	if err == nil {
		t.Fatal("expected download --gui to be rejected")
	}
	if !strings.Contains(err.Error(), "gui") {
		t.Fatalf("expected error to mention gui, got %v", err)
	}
}

func TestMain_InstallAllFlagBindsWithoutTarget(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"install", "--all"})
	if err != nil {
		t.Fatalf("expected install --all to parse, got %v", err)
	}
	if len(calls) != 1 || calls[0].name != "install" {
		t.Fatalf("unexpected routed call: %#v", calls)
	}
	opts, ok := calls[0].options.(*InstallOptions)
	if !ok {
		t.Fatalf("expected InstallOptions, got %T", calls[0].options)
	}
	if !opts.InstallAll {
		t.Fatalf("expected install all flag to be true")
	}
	if len(opts.Targets) != 0 {
		t.Fatalf("expected install --all to omit target, got %#v", opts.Targets)
	}
}

func TestMain_DownloadAndAddRejectRemovedAllFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"download", []string{"download", "--all", "inhere/markview"}},
		{"add", []string{"add", "--all", "inhere/markview"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(func(string, any) error { return nil }, &stdout, &stderr).RunWithArgs(tt.args)
			if err == nil {
				t.Fatalf("expected %s --all to be rejected", tt.name)
			}
			if !strings.Contains(err.Error(), "all") {
				t.Fatalf("expected error to mention all, got %v", err)
			}
		})
	}
}

func TestMain_InstallRejectsRemovedCacheDirFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Main([]string{"install", "--cache-dir", "~/.cache/eget", "inhere/markview"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected parse error for removed --cache-dir flag")
	}
	if !strings.Contains(err.Error(), "cache-dir") {
		t.Fatalf("expected error to mention cache-dir, got %v", err)
	}
}

func TestMain_InstallRejectsFlagsAfterTarget(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Main([]string{"install", "inhere/markview", "--tag", "nightly"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected parse error for trailing flags after target")
	}
	if !strings.Contains(err.Error(), "flags must appear before arguments") {
		t.Fatalf("expected trailing-flag error, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected stderr to be empty, got %q", stderr.String())
	}
}

func TestMain_ConfigSubcommandsRouteToConfigCommand(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      ConfigOptions
		wantCalls int
	}{
		{
			name:      "init",
			args:      []string{"config", "init"},
			want:      ConfigOptions{Action: "init"},
			wantCalls: 1,
		},
		{
			name:      "list",
			args:      []string{"config", "list"},
			want:      ConfigOptions{Action: "list"},
			wantCalls: 1,
		},
		{
			name:      "list alias",
			args:      []string{"config", "ls"},
			want:      ConfigOptions{Action: "list"},
			wantCalls: 1,
		},
		{
			name:      "get",
			args:      []string{"config", "get", "global.target"},
			want:      ConfigOptions{Action: "get", Key: "global.target"},
			wantCalls: 1,
		},
		{
			name:      "set",
			args:      []string{"config", "set", "global.target", "~/.local/bin"},
			want:      ConfigOptions{Action: "set", Key: "global.target", Value: "~/.local/bin"},
			wantCalls: 1,
		},
		{
			name:      "top alias",
			args:      []string{"cfg", "list"},
			want:      ConfigOptions{Action: "list"},
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			assert.NoErr(t, err)
			assert.Eq(t, tt.wantCalls, len(calls))
			assert.Eq(t, "config", calls[0].name)

			opts, ok := calls[0].options.(*ConfigOptions)
			assert.True(t, ok)
			assert.Eq(t, tt.want.Action, opts.Action)
			assert.Eq(t, tt.want.Key, opts.Key)
			assert.Eq(t, tt.want.Value, opts.Value)
		})
	}
}

func TestMain_ConfigWithoutSubcommandShowsHelp(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"config"})
	assert.NoErr(t, err)
	assert.Eq(t, 0, len(calls))

	help := stdout.String()
	if !strings.Contains(help, "Available Commands") && !strings.Contains(help, "Usage") {
		t.Fatalf("expected config help output, got %q", help)
	}
	if !strings.Contains(help, "init") || !strings.Contains(help, "get") || !strings.Contains(help, "set") {
		t.Fatalf("expected config subcommands in help output, got %q", help)
	}
}

func TestMain_ConfigHelpFlagShowsSubcommandHelp(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"config", "--help"})
	assert.NoErr(t, err)
	assert.Eq(t, 0, len(calls))

	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "init") || !strings.Contains(help, "get") || !strings.Contains(help, "set") {
		t.Fatalf("expected config subcommands in help output, got %q", help)
	}
}

func TestMain_ConfigSubcommandRejectsUnknownFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(func(string, any) error { return nil }, &stdout, &stderr).
		RunWithArgs([]string{"config", "get", "--bad", "global.target"})
	if err == nil {
		t.Fatal("expected config get --bad to be rejected")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("expected error to mention bad flag, got %v", err)
	}
}

func TestMain_SDKWithoutSubcommandShowsHelp(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"sdk"})
	assert.NoErr(t, err)
	assert.Eq(t, 0, len(calls))

	help := stdout.String()
	if !strings.Contains(help, "install") || !strings.Contains(help, "index") {
		t.Fatalf("expected sdk help output, got %q", help)
	}
}

func TestMain_SDKRoutesAndBindsOptions(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCmd    string
		assertOpts func(*testing.T, any)
	}{
		{
			name:    "install exact",
			args:    []string{"sdk", "install", "go@1.21.1"},
			wantCmd: "sdk.install",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKInstallOptions)
				assert.True(t, ok)
				assert.Eq(t, []string{"go@1.21.1"}, opts.Targets)
				assert.False(t, opts.Force)
			},
		},
		{
			name:    "install force multiple",
			args:    []string{"sdk", "install", "--force", "go@1.21.1", "node:20.11.1"},
			wantCmd: "sdk.install",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKInstallOptions)
				assert.True(t, ok)
				assert.Eq(t, []string{"go@1.21.1", "node:20.11.1"}, opts.Targets)
				assert.True(t, opts.Force)
			},
		},
		{
			name:    "list all",
			args:    []string{"sdk", "list"},
			wantCmd: "sdk.list",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKListOptions)
				assert.True(t, ok)
				assert.Eq(t, "", opts.Name)
				assert.False(t, opts.JSON)
			},
		},
		{
			name:    "list json name",
			args:    []string{"sdk", "list", "--json", "go"},
			wantCmd: "sdk.list",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKListOptions)
				assert.True(t, ok)
				assert.Eq(t, "go", opts.Name)
				assert.True(t, opts.JSON)
			},
		},
		{
			name:    "remove",
			args:    []string{"sdk", "remove", "go@1.21.1"},
			wantCmd: "sdk.remove",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKRemoveOptions)
				assert.True(t, ok)
				assert.Eq(t, "go@1.21.1", opts.Target)
			},
		},
		{
			name:    "search keywords",
			args:    []string{"sdk", "search", "go", "1.22", "amd64", "^windows"},
			wantCmd: "sdk.search",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKSearchOptions)
				assert.True(t, ok)
				assert.Eq(t, "go", opts.Name)
				assert.Eq(t, []string{"1.22", "amd64", "^windows"}, opts.Keywords)
				assert.Eq(t, 20, opts.Number)
				assert.False(t, opts.JSON)
			},
		},
		{
			name:    "search json",
			args:    []string{"sdk", "search", "--json", "--number", "5", "--sort", "desc", "node", "REG:^22"},
			wantCmd: "sdk.search",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKSearchOptions)
				assert.True(t, ok)
				assert.Eq(t, "node", opts.Name)
				assert.Eq(t, []string{"REG:^22"}, opts.Keywords)
				assert.Eq(t, 5, opts.Number)
				assert.Eq(t, "desc", opts.Sort)
				assert.True(t, opts.JSON)
			},
		},
		{
			name:    "search unlimited short number",
			args:    []string{"sdk", "search", "-n", "0", "node", "20"},
			wantCmd: "sdk.search",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKSearchOptions)
				assert.True(t, ok)
				assert.Eq(t, "node", opts.Name)
				assert.Eq(t, []string{"20"}, opts.Keywords)
				assert.Eq(t, 0, opts.Number)
			},
		},
		{
			name:    "index refresh name",
			args:    []string{"sdk", "index", "refresh", "go"},
			wantCmd: "sdk.index.refresh",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKIndexOptions)
				assert.True(t, ok)
				assert.Eq(t, "refresh", opts.Action)
				assert.Eq(t, "go", opts.Name)
			},
		},
		{
			name:    "index refresh all",
			args:    []string{"sdk", "index", "refresh", "--all"},
			wantCmd: "sdk.index.refresh",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKIndexOptions)
				assert.True(t, ok)
				assert.True(t, opts.All)
			},
		},
		{
			name:    "index refresh alias",
			args:    []string{"sdk", "idx", "build", "node"},
			wantCmd: "sdk.index.refresh",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKIndexOptions)
				assert.True(t, ok)
				assert.Eq(t, "refresh", opts.Action)
				assert.Eq(t, "node", opts.Name)
			},
		},
		{
			name:    "index list alias",
			args:    []string{"sdk", "idx", "ls"},
			wantCmd: "sdk.index.list",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKIndexOptions)
				assert.True(t, ok)
				assert.Eq(t, "list", opts.Action)
			},
		},
		{
			name:    "index clear all",
			args:    []string{"sdk", "index", "clear", "--all"},
			wantCmd: "sdk.index.clear",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKIndexOptions)
				assert.True(t, ok)
				assert.True(t, opts.All)
			},
		},
		{
			name:    "index show name",
			args:    []string{"sdk", "index", "show", "go"},
			wantCmd: "sdk.index.show",
			assertOpts: func(t *testing.T, options any) {
				opts, ok := options.(*SDKIndexOptions)
				assert.True(t, ok)
				assert.Eq(t, "show", opts.Action)
				assert.Eq(t, "go", opts.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			assert.NoErr(t, err)
			assert.Eq(t, 1, len(calls))
			assert.Eq(t, tt.wantCmd, calls[0].name)
			tt.assertOpts(t, calls[0].options)
		})
	}
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

func TestMain_ListRoutesToListCommand(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"list"})
	if err != nil {
		t.Fatalf("expected list command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}
	if calls[0].name != "list" {
		t.Fatalf("expected command list, got %q", calls[0].name)
	}

	if _, ok := calls[0].options.(*ListOptions); !ok {
		t.Fatalf("expected ListOptions, got %T", calls[0].options)
	}
}

func TestMain_ListOutdatedBindsOption(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"list", "--outdated"})
	if err != nil {
		t.Fatalf("expected list --outdated command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}

	opts, ok := calls[0].options.(*ListOptions)
	if !ok {
		t.Fatalf("expected ListOptions, got %T", calls[0].options)
	}
	if !opts.Outdated {
		t.Fatalf("expected outdated flag to be true")
	}
}

func TestMain_ShowBindsTarget(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"show", "fzf"})
	if err != nil {
		t.Fatalf("expected show command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}
	if calls[0].name != "show" {
		t.Fatalf("expected command show, got %q", calls[0].name)
	}
	opts, ok := calls[0].options.(*ShowOptions)
	if !ok {
		t.Fatalf("expected ShowOptions, got %T", calls[0].options)
	}
	assert.Eq(t, "fzf", opts.Target)
}

func TestMain_ListAllBindsOption(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"list", "--all"})
	if err != nil {
		t.Fatalf("expected list --all command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}

	opts, ok := calls[0].options.(*ListOptions)
	if !ok {
		t.Fatalf("expected ListOptions, got %T", calls[0].options)
	}
	if !opts.All {
		t.Fatalf("expected all flag to be true")
	}
}

func TestMain_ListAllShortBindsOption(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"list", "-a"})
	if err != nil {
		t.Fatalf("expected list -a command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}

	opts, ok := calls[0].options.(*ListOptions)
	if !ok {
		t.Fatalf("expected ListOptions, got %T", calls[0].options)
	}
	if !opts.All {
		t.Fatalf("expected all flag to be true")
	}
}

func TestMain_ListNoInstalledBindsOption(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "long", args: []string{"list", "--no-installed"}},
		{name: "short", args: []string{"list", "--ni"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(name string, options any) error {
				calls = append(calls, commandCall{name: name, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs(tt.args)
			if err != nil {
				t.Fatalf("expected list no-installed command to parse, got %v", err)
			}
			if len(calls) != 1 {
				t.Fatalf("expected one handler call, got %d", len(calls))
			}

			opts, ok := calls[0].options.(*ListOptions)
			if !ok {
				t.Fatalf("expected ListOptions, got %T", calls[0].options)
			}
			if !opts.NoInstalled {
				t.Fatalf("expected no-installed flag to be true")
			}
		})
	}
}

func TestMain_ListGUIBindsOption(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"list", "--gui"})
	if err != nil {
		t.Fatalf("expected list --gui command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}

	opts, ok := calls[0].options.(*ListOptions)
	if !ok {
		t.Fatalf("expected ListOptions, got %T", calls[0].options)
	}
	if !opts.GUI {
		t.Fatalf("expected gui flag to be true")
	}
}

func TestMain_ListInfoBindsOption(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"list", "--info", "chlog"})
	if err != nil {
		t.Fatalf("expected list --info command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}

	opts, ok := calls[0].options.(*ListOptions)
	if !ok {
		t.Fatalf("expected ListOptions, got %T", calls[0].options)
	}
	if opts.Info != "chlog" {
		t.Fatalf("expected info option chlog, got %q", opts.Info)
	}
}

func TestMain_QueryRoutesAndBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"query", "--action", "releases", "--limit", "5", "--json", "owner/repo"})
	if err != nil {
		t.Fatalf("expected query command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}
	if calls[0].name != "query" {
		t.Fatalf("expected command query, got %q", calls[0].name)
	}

	opts, ok := calls[0].options.(*QueryOptions)
	if !ok {
		t.Fatalf("expected QueryOptions, got %T", calls[0].options)
	}
	if opts.Action != "releases" || opts.Limit != 5 || !opts.JSON || opts.Target != "owner/repo" {
		t.Fatalf("unexpected query options: %#v", opts)
	}
}

func TestMain_QueryAliasRoutes(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"q", "owner/repo"})
	if err != nil {
		t.Fatalf("expected query alias to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}
	if calls[0].name != "query" {
		t.Fatalf("expected command query, got %q", calls[0].name)
	}
}

func TestMain_UninstallRoutesToUninstallCommandAndAliases(t *testing.T) {
	for _, name := range []string{"uninstall", "uni", "rm"} {
		t.Run(name, func(t *testing.T) {
			calls := make([]commandCall, 0, 1)
			handler := func(cmdName string, options any) error {
				calls = append(calls, commandCall{name: cmdName, options: options})
				return nil
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{name, "fzf"})
			if err != nil {
				t.Fatalf("expected %s command to parse, got %v", name, err)
			}
			if len(calls) != 1 {
				t.Fatalf("expected one handler call, got %d", len(calls))
			}
			if calls[0].name != "uninstall" {
				t.Fatalf("expected command uninstall, got %q", calls[0].name)
			}

			opts, ok := calls[0].options.(*UninstallOptions)
			if !ok {
				t.Fatalf("expected UninstallOptions, got %T", calls[0].options)
			}
			if opts.Target != "fzf" {
				t.Fatalf("expected uninstall target fzf, got %q", opts.Target)
			}
		})
	}
}

func TestApp_RunWithArgsDoesNotLeakCommandStateAcrossRuns(t *testing.T) {
	calls := make([]commandCall, 0, 4)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newApp(handler, &stdout, &stderr)

	if err := app.RunWithArgs([]string{"update", "foo"}); err != nil {
		t.Fatalf("expected first update run to succeed, got %v", err)
	}
	if err := app.RunWithArgs([]string{"update"}); err != nil {
		t.Fatalf("expected second update run to succeed, got %v", err)
	}
	if err := app.RunWithArgs([]string{"install", "--tag", "nightly", "inhere/markview"}); err != nil {
		t.Fatalf("expected first install run to succeed, got %v", err)
	}
	if err := app.RunWithArgs([]string{"install", "inhere/markview"}); err != nil {
		t.Fatalf("expected second install run to succeed, got %v", err)
	}

	if len(calls) != 4 {
		t.Fatalf("expected four handler calls, got %d", len(calls))
	}

	updateFirst, ok := calls[0].options.(*UpdateOptions)
	if !ok {
		t.Fatalf("expected first update options, got %T", calls[0].options)
	}
	updateSecond, ok := calls[1].options.(*UpdateOptions)
	if !ok {
		t.Fatalf("expected second update options, got %T", calls[1].options)
	}
	if len(updateFirst.Targets) != 1 || updateFirst.Targets[0] != "foo" {
		t.Fatalf("expected first update target foo, got %#v", updateFirst.Targets)
	}
	if len(updateSecond.Targets) != 0 {
		t.Fatalf("expected second update target to reset, got %#v", updateSecond.Targets)
	}

	installFirst, ok := calls[2].options.(*InstallOptions)
	if !ok {
		t.Fatalf("expected first install options, got %T", calls[2].options)
	}
	installSecond, ok := calls[3].options.(*InstallOptions)
	if !ok {
		t.Fatalf("expected second install options, got %T", calls[3].options)
	}
	if installFirst.Tag != "nightly" {
		t.Fatalf("expected first install tag nightly, got %q", installFirst.Tag)
	}
	if installSecond.Tag != "" {
		t.Fatalf("expected second install tag to reset, got %q", installSecond.Tag)
	}
}

func TestMain_UpdateCheckBindsOption(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"up", "--check"})
	if err != nil {
		t.Fatalf("expected update --check command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}
	if calls[0].name != "update" {
		t.Fatalf("expected command update, got %q", calls[0].name)
	}

	opts, ok := calls[0].options.(*UpdateOptions)
	if !ok {
		t.Fatalf("expected UpdateOptions, got %T", calls[0].options)
	}
	if !opts.Check {
		t.Fatalf("expected check flag to be true")
	}
}

func TestMain_SearchRoutesAndBindsOptions(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{
		"search", "--limit", "10", "--sort", "stars", "--order", "desc", "--json",
		"keyword", "user:junegunn", "language:go",
	})
	if err != nil {
		t.Fatalf("expected search command to parse, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one handler call, got %d", len(calls))
	}
	if calls[0].name != "search" {
		t.Fatalf("expected command search, got %q", calls[0].name)
	}

	opts, ok := calls[0].options.(*SearchOptions)
	if !ok {
		t.Fatalf("expected SearchOptions, got %T", calls[0].options)
	}
	if opts.Limit != 10 || opts.Sort != "stars" || opts.Order != "desc" || !opts.JSON {
		t.Fatalf("unexpected search flags: %#v", opts)
	}
	if opts.Keyword != "keyword" {
		t.Fatalf("expected keyword, got %q", opts.Keyword)
	}
	if len(opts.Extras) != 2 || opts.Extras[0] != "user:junegunn" || opts.Extras[1] != "language:go" {
		t.Fatalf("unexpected search extras: %#v", opts.Extras)
	}
}

func TestApp_RunWithArgsDoesNotLeakSearchStateAcrossRuns(t *testing.T) {
	calls := make([]commandCall, 0, 2)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newApp(handler, &stdout, &stderr)

	if err := app.RunWithArgs([]string{"search", "--limit", "10", "--json", "keyword", "language:go"}); err != nil {
		t.Fatalf("expected first search run to succeed, got %v", err)
	}
	if err := app.RunWithArgs([]string{"search", "second"}); err != nil {
		t.Fatalf("expected second search run to succeed, got %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected two handler calls, got %d", len(calls))
	}

	first, ok := calls[0].options.(*SearchOptions)
	if !ok {
		t.Fatalf("expected first search options, got %T", calls[0].options)
	}
	second, ok := calls[1].options.(*SearchOptions)
	if !ok {
		t.Fatalf("expected second search options, got %T", calls[1].options)
	}

	if first.Limit != 10 || !first.JSON || first.Keyword != "keyword" || len(first.Extras) != 1 {
		t.Fatalf("unexpected first search options: %#v", first)
	}
	if second.Limit != 10 || second.JSON || second.Keyword != "second" || len(second.Extras) != 0 {
		t.Fatalf("expected second search options reset, got %#v", second)
	}
}
