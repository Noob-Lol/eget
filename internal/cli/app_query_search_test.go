package cli

import (
	"bytes"
	"testing"
)

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
