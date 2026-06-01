package cli

import (
	"bytes"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

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

func TestMain_InfoAliasBindsShowTarget(t *testing.T) {
	calls := make([]commandCall, 0, 1)
	handler := func(name string, options any) error {
		calls = append(calls, commandCall{name: name, options: options})
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := newApp(handler, &stdout, &stderr).RunWithArgs([]string{"info", "fzf"})
	assert.NoErr(t, err)
	assert.Eq(t, 1, len(calls))
	assert.Eq(t, "show", calls[0].name)
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
