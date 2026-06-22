package cli

import "github.com/gookit/gcli/v3"

type UninstallOptions struct {
	Target  string
	Targets []string
	Yes     bool
	Purge   bool
}

func newUninstallCmd(handler CommandHandler) (*gcli.Command, func()) {
	opts := &UninstallOptions{}
	cmd := gcli.NewCommand("uninstall", "Uninstall a managed package or repo")
	cmd.Aliases = []string{"uni", "rm", "remove"}
	cmd.Config = func(c *gcli.Command) {
		c.BoolOpt(&opts.Yes, "yes", "y", false, "Confirm removal without prompting")
		c.BoolOpt(&opts.Purge, "purge", "", false, "Also remove the package definition from config")
		c.AddArg("target", "Package name or repo to uninstall", true, true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		targetArgs := append(c.Arg("target").Strings(), args...)
		if err := validateNoFlagArgs(targetArgs); err != nil {
			return err
		}
		opts.Targets = splitTargets(targetArgs)
		if len(opts.Targets) > 0 {
			opts.Target = opts.Targets[0]
		}
		snapshot := *opts
		snapshot.Targets = append([]string(nil), opts.Targets...)
		return handler("uninstall", &snapshot)
	}
	return cmd, func() {
		*opts = UninstallOptions{}
	}
}
