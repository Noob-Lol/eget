package cli

import "github.com/gookit/gcli/v3"

type ShowOptions struct {
	Target string
}

func newShowCmd(handler CommandHandler) (*gcli.Command, func()) {
	opts := &ShowOptions{}
	cmd := gcli.NewCommand("show", "Show package details")
	cmd.Config = func(c *gcli.Command) {
		c.AddArg("target", "Package name or repo to show", true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		target := c.Arg("target").String()
		if err := validateNoFlagArgs(args); err != nil {
			return err
		}
		opts.Target = target
		snapshot := *opts
		return handler("show", &snapshot)
	}
	return cmd, func() {
		*opts = ShowOptions{}
	}
}
