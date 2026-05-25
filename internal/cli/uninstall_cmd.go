package cli

import "github.com/gookit/gcli/v3"

type UninstallOptions struct {
	Target string
	Yes    bool
}

func newUninstallCmd(handler CommandHandler) (*gcli.Command, func()) {
	opts := &UninstallOptions{}
	cmd := gcli.NewCommand("uninstall", "Uninstall a managed package or repo")
	cmd.Aliases = []string{"uni", "rm", "remove"}
	cmd.Config = func(c *gcli.Command) {
		c.BoolOpt(&opts.Yes, "yes", "y", false, "Confirm removal without prompting")
		c.AddArg("target", "Package name or repo to uninstall", true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Target = c.Arg("target").String()
		if err := validateNoFlagArgs(append([]string{opts.Target}, args...)); err != nil {
			return err
		}
		snapshot := *opts
		return handler("uninstall", &snapshot)
	}
	return cmd, func() {
		*opts = UninstallOptions{}
	}
}
