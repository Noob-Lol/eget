package cli

import "github.com/gookit/gcli/v3"

type ConfigOptions struct {
	Action string
	Key    string
	Value  string
}

func newConfigCmd(handler CommandHandler) (*gcli.Command, func()) {
	opts := &ConfigOptions{}
	cmd := gcli.NewCommand("config", "Manage configuration")
	cmd.Aliases = []string{"cfg"}
	cmd.Help = `<info>Config actions</>:
  init                Initialize the config file with default values
  list | ls           Print current config values and file status
  get KEY             Print one config value
  set KEY VALUE       Update one config value

<info>Examples</>:
  eget config init
  eget config list
  eget config get global.target
  eget config set global.target ~/.local/bin`

	cmd.Config = func(c *gcli.Command) {
		c.AddArg("action", "Config action: init, list, ls, get, set", false)
		c.AddArg("key", "Config key", false)
		c.AddArg("value", "Config value for set", false)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Action = c.Arg("action").String()
		opts.Key = c.Arg("key").String()
		opts.Value = c.Arg("value").String()
		if err := validateNoFlagArgs(args); err != nil {
			return err
		}
		snapshot := *opts
		return handler("config", &snapshot)
	}
	return cmd, func() {
		*opts = ConfigOptions{}
	}
}
