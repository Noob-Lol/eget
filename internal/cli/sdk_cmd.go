package cli

import (
	"fmt"

	"github.com/gookit/gcli/v3"
)

type SDKInstallOptions struct {
	Targets []string
	Force   bool
}

type SDKListOptions struct {
	Name string
	JSON bool
}

type SDKRemoveOptions struct {
	Target string
}

type SDKSearchOptions struct {
	Name     string
	Keywords []string
	JSON     bool
	Number   int
}

type SDKIndexOptions struct {
	Action string
	Name   string
	All    bool
	JSON   bool
}

func newSDKCmd(handler CommandHandler) (*gcli.Command, func()) {
	installOpts := &SDKInstallOptions{}
	listOpts := &SDKListOptions{}
	removeOpts := &SDKRemoveOptions{}
	searchOpts := &SDKSearchOptions{Number: 20}
	indexOpts := &SDKIndexOptions{}
	cmd := gcli.NewCommand("sdk", "Download and manage SDK archives")
	cmd.Help = `<info>Examples</>:
  eget sdk install go@1.22.0
  eget sdk install --force go:1.22 node:20.11.1
  eget sdk list
  eget sdk remove go@1.22.0
  eget sdk search go 1.22 amd64 ^windows
  eget sdk index refresh go`
	cmd.Subs = []*gcli.Command{
		newSDKInstallCmd(installOpts, handler),
		newSDKListCmd(listOpts, handler),
		newSDKRemoveCmd(removeOpts, handler),
		newSDKSearchCmd(searchOpts, handler),
		newSDKIndexCmd(indexOpts, handler),
	}
	return cmd, func() {
		*installOpts = SDKInstallOptions{}
		*listOpts = SDKListOptions{}
		*removeOpts = SDKRemoveOptions{}
		*searchOpts = SDKSearchOptions{Number: 20}
		*indexOpts = SDKIndexOptions{}
	}
}

func newSDKInstallCmd(opts *SDKInstallOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("install", "Install SDK target(s)")
	cmd.Aliases = []string{"i", "ins"}
	cmd.Config = func(c *gcli.Command) {
		c.BoolOpt(&opts.Force, "force", "f", false, "Remove existing SDK directory before installing")
		c.AddArg("target", "SDK target(s), for example go@1.22.0 or node:20.11.1", true, true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		targetArgs := append(c.Arg("target").Strings(), args...)
		if err := validateNoFlagArgs(targetArgs); err != nil {
			return err
		}
		opts.Targets = splitTargets(targetArgs)
		snapshot := *opts
		snapshot.Targets = append([]string(nil), opts.Targets...)
		return handler("sdk.install", &snapshot)
	}
	return cmd
}

func newSDKListCmd(opts *SDKListOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("list", "List installed SDK versions")
	cmd.Aliases = []string{"ls"}
	cmd.Config = func(c *gcli.Command) {
		c.BoolOpt(&opts.JSON, "json", "j", false, "Output as JSON")
		c.AddArg("name", "SDK name filter", false)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		name := c.Arg("name").String()
		if err := validateNoFlagArgs(append([]string{name}, args...)); err != nil {
			return err
		}
		opts.Name = name
		snapshot := *opts
		return handler("sdk.list", &snapshot)
	}
	return cmd
}

func newSDKRemoveCmd(opts *SDKRemoveOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("remove", "Remove an installed SDK version")
	cmd.Aliases = []string{"rm"}
	cmd.Config = func(c *gcli.Command) {
		c.AddArg("target", "SDK target with explicit version", true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Target = c.Arg("target").String()
		if err := validateNoFlagArgs(append([]string{opts.Target}, args...)); err != nil {
			return err
		}
		snapshot := *opts
		return handler("sdk.remove", &snapshot)
	}
	return cmd
}

func newSDKSearchCmd(opts *SDKSearchOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("search", "Search SDK index cache")
	cmd.Config = func(c *gcli.Command) {
		c.BoolOpt(&opts.JSON, "json", "j", false, "Output as JSON")
		c.IntOpt(&opts.Number, "number", "n", 20, "Maximum search results, <= 0 means unlimited")
		c.AddArg("name", "SDK name", true)
		c.AddArg("keyword", "Search keyword(s), prefix with ^ to exclude", false, true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Name = c.Arg("name").String()
		keywords := append(c.Arg("keyword").Strings(), args...)
		if err := validateNoFlagArgs(append([]string{opts.Name}, keywords...)); err != nil {
			return err
		}
		opts.Keywords = append([]string(nil), keywords...)
		snapshot := *opts
		snapshot.Keywords = append([]string(nil), opts.Keywords...)
		return handler("sdk.search", &snapshot)
	}
	return cmd
}

func newSDKIndexCmd(opts *SDKIndexOptions, handler CommandHandler) *gcli.Command {
	cmd := gcli.NewCommand("index", "Manage SDK index cache")
	cmd.Aliases = []string{"idx"}
	cmd.Subs = []*gcli.Command{
		newSDKIndexActionCmd("list", opts, handler, func(c *gcli.Command) {
			c.Aliases = []string{"ls"}
		}),
		newSDKIndexActionCmd("show", opts, handler, nil),
		newSDKIndexActionCmd("refresh", opts, handler, func(c *gcli.Command) {
			c.Aliases = []string{"build"}
		}),
		newSDKIndexActionCmd("clear", opts, handler, nil),
	}
	return cmd
}

func newSDKIndexActionCmd(action string, opts *SDKIndexOptions, handler CommandHandler, configure func(*gcli.Command)) *gcli.Command {
	cmd := gcli.NewCommand(action, "Run sdk index "+action)
	if configure != nil {
		configure(cmd)
	}
	cmd.Config = func(c *gcli.Command) {
		if action == "refresh" || action == "clear" {
			c.BoolOpt(&opts.All, "all", "a", false, "Apply to all SDK indexes")
		}
		if action == "list" {
			c.BoolOpt(&opts.JSON, "json", "j", false, "Output as JSON")
		}
		if action == "show" || action == "refresh" || action == "clear" {
			c.AddArg("name", "SDK name", action == "show")
		}
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Action = action
		if action == "show" || action == "refresh" || action == "clear" {
			opts.Name = c.Arg("name").String()
		}
		if err := validateNoFlagArgs(append([]string{opts.Name}, args...)); err != nil {
			return err
		}
		if (action == "refresh" || action == "clear") && ((opts.Name == "") == !opts.All) {
			return fmt.Errorf("sdk index %s requires exactly one of <name> or --all", action)
		}
		snapshot := *opts
		return handler("sdk.index."+action, &snapshot)
	}
	return cmd
}
