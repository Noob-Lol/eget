package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gookit/color"
	"github.com/gookit/gcli/v3"
)

var (
	version   string
	gitHash   string
	buildTime string
)

var (
	ErrNotImplemented = errors.New("not implemented")
)

type CommandHandler func(name string, options any) error

type App struct {
	inner     *gcli.App
	commands  []*gcli.Command
	resetters []func()
	verbose   *bool
	lastErr   error
	stdout    io.Writer
}

// SetBuildInfo sets the build information for the application.
func SetBuildInfo(versionStr, gitHashStr, buildTimeStr string) {
	version = versionStr
	gitHash = gitHashStr
	buildTime = normalizeBuildTime(buildTimeStr)
}

func normalizeBuildTime(value string) string {
	for _, layout := range []string{
		compactTimeLayout,
		time.RFC3339,
		"2006/01/02-15:04:05",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format(compactTimeLayout)
		}
	}
	return value
}

var cliApp *App

func Main(args []string, stdout, stderr io.Writer) error {
	var service *cliService
	var serviceErr error
	cliApp = newApp(func(name string, options any) error {
		if service == nil && serviceErr == nil {
			service, serviceErr = newCLIService()
		}
		if serviceErr != nil {
			return serviceErr
		}
		if service == nil {
			return ErrNotImplemented
		}
		service.stderr = stderr
		configureVerbose(cliApp.Verbose(), stderr)
		return service.handle(name, options)
	}, stdout, stderr)
	return cliApp.RunWithArgs(args)
}

func newApp(handler CommandHandler, stdout, stderr io.Writer) *App {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if handler == nil {
		handler = func(name string, options any) error {
			_ = name
			_ = options
			return ErrNotImplemented
		}
	}

	inner := gcli.NewApp(gcli.NotExitOnEnd())
	inner.Name = "eget"
	inner.Desc = "Easy install and download tools from GitHub, SourceForge and more"
	inner.Version = buildVersionString()
	verbose := false
	app := &App{inner: inner, verbose: &verbose, stdout: stdout}
	cliApp = app
	inner.On(gcli.EvtAppRunError, func(ctx *gcli.HookCtx) bool {
		if errV := ctx.Get("err"); errV != nil {
			if err, ok := errV.(error); ok {
				app.lastErr = err
			}
		}
		return true
	})
	app.add(newInstallCmd(handler))
	app.add(newDownloadCmd(handler))
	app.add(newSDKCmd(handler))
	app.add(newAddCmd(handler))
	app.add(newUninstallCmd(handler))
	app.add(newListCmd(handler))
	app.add(newShowCmd(handler))
	app.add(newUpdateCmd(handler))
	app.add(newConfigCmd(handler))
	app.add(newQueryCmd(handler))
	app.add(newSearchCmd(handler))
	return app
}

func (a *App) add(cmd *gcli.Command, reset func()) {
	a.inner.Add(cmd)
	a.commands = append(a.commands, cmd)
	a.resetters = append(a.resetters, reset)
}

func (a *App) RunWithArgs(args []string) error {
	a.lastErr = nil
	for _, reset := range a.resetters {
		reset()
	}
	for _, cmd := range a.commands {
		for _, arg := range cmd.Args() {
			arg.Reset()
		}
	}
	args = a.consumeVerboseArgs(args)
	if err := validateKnownFlags(args); err != nil {
		return err
	}
	color.SetOutput(a.stdout)
	defer color.ResetOutput()
	a.inner.Run(args)
	return a.lastErr
}

func (a *App) Verbose() bool {
	return a.verbose != nil && *a.verbose
}

func (a *App) consumeVerboseArgs(args []string) []string {
	if a.verbose != nil {
		*a.verbose = false
	}
	if len(args) == 0 {
		return args
	}
	cleaned := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "-v" || arg == "--verbose" {
			if a.verbose != nil {
				*a.verbose = true
			}
			continue
		}
		cleaned = append(cleaned, arg)
	}
	return cleaned
}

func buildVersionString() string {
	if gitHash == "" && buildTime == "" {
		return version
	}
	return fmt.Sprintf("%s (%s, %s)", version, gitHash, buildTime)
}

func validateNoFlagArgs(args []string) error {
	for _, arg := range args {
		if len(arg) > 0 && arg[0] == '-' {
			return fmt.Errorf("flags must appear before arguments: %s", arg)
		}
	}
	return nil
}

type flagSpec struct {
	bools  map[string]bool
	values map[string]bool
	subs   map[string]flagSpec
}

var commandAliases = map[string]string{
	"i":   "install",
	"ins": "install",
	"dl":  "download",
	"uni": "uninstall",
	"rm":  "uninstall",
	"ls":  "list",
	"up":  "update",
	"cfg": "config",
	"q":   "query",
}

var commandFlagSpecs = map[string]flagSpec{
	"install": {
		bools:  setOf("source", "extract-all", "ea", "all", "gui", "quiet", "add"),
		values: setOf("tag", "system", "to", "file", "asset", "a", "name", "fallback-versions", "chunk", "batch"),
	},
	"download": {
		bools:  setOf("source", "extract-all", "ea", "quiet"),
		values: setOf("tag", "system", "to", "file", "asset", "a", "fallback-versions", "chunk"),
	},
	"add": {
		bools:  setOf("source", "extract-all", "ea", "gui", "quiet"),
		values: setOf("name", "tag", "system", "to", "file", "asset", "source", "chunk"),
	},
	"list": {
		bools:  setOf("outdated", "old", "all", "a", "gui", "no-installed", "ni"),
		values: setOf("info", "i"),
	},
	"update": {
		bools:  setOf("all", "A", "check", "dry-run", "interactive", "source", "quiet"),
		values: setOf("tag", "system", "to", "file", "asset", "a", "chunk", "batch"),
	},
	"query": {
		bools:  setOf("json", "j", "prerelease", "p"),
		values: setOf("action", "a", "tag", "t", "limit", "l"),
	},
	"sdk": {
		subs: map[string]flagSpec{
			"install": {
				bools: setOf("force", "f"),
			},
			"i": {
				bools: setOf("force", "f"),
			},
			"ins": {
				bools: setOf("force", "f"),
			},
			"list": {
				bools: setOf("json", "j"),
			},
			"ls": {
				bools: setOf("json", "j"),
			},
			"remove": {},
			"rm":     {},
			"search": {
				bools:  setOf("json", "j"),
				values: setOf("number", "n", "sort"),
			},
			"index": {
				subs: map[string]flagSpec{
					"list": {
						bools: setOf("json", "j"),
					},
					"ls": {
						bools: setOf("json", "j"),
					},
					"show": {},
					"refresh": {
						bools: setOf("all", "a"),
					},
					"build": {
						bools: setOf("all", "a"),
					},
					"clear": {
						bools: setOf("all", "a"),
					},
				},
			},
			"idx": {
				subs: map[string]flagSpec{
					"list": {
						bools: setOf("json", "j"),
					},
					"ls": {
						bools: setOf("json", "j"),
					},
					"show": {},
					"refresh": {
						bools: setOf("all", "a"),
					},
					"build": {
						bools: setOf("all", "a"),
					},
					"clear": {
						bools: setOf("all", "a"),
					},
				},
			},
		},
	},
	"search": {
		bools:  setOf("json", "j"),
		values: setOf("sort", "order", "limit", "l"),
	},
	"show":      {},
	"uninstall": {},
	"config": {
		subs: map[string]flagSpec{
			"init": {},
			"list": {},
			"ls":   {},
			"get":  {},
			"set":  {},
		},
	},
}

func setOf(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

func validateKnownFlags(args []string) error {
	cmdName, start := findCommandArg(args)
	if cmdName == "" {
		return nil
	}
	spec, ok := commandFlagSpecs[cmdName]
	if !ok {
		return nil
	}
	if len(spec.subs) > 0 && start < len(args) {
		subName := args[start]
		if !strings.HasPrefix(subName, "-") {
			if subSpec, ok := spec.subs[subName]; ok {
				spec = subSpec
				start++
			}
		}
	}
	if len(spec.subs) > 0 && start < len(args) {
		subName := args[start]
		if !strings.HasPrefix(subName, "-") {
			if subSpec, ok := spec.subs[subName]; ok {
				spec = subSpec
				start++
			}
		}
	}
	for i := start; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return nil
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return nil
		}
		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		if name == "help" || name == "h" {
			continue
		}
		if spec.bools[name] {
			continue
		}
		if spec.values[name] {
			if !strings.Contains(arg, "=") {
				i++
			}
			continue
		}
		return fmt.Errorf("option provided but not defined: %s", arg)
	}
	return nil
}

func findCommandArg(args []string) (string, int) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-v", "--verbose", "-V", "--version", "-h", "--help":
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if real, ok := commandAliases[arg]; ok {
			arg = real
		}
		return arg, i + 1
	}
	return "", 0
}
