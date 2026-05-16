package cli

import "github.com/gookit/gcli/v3"

type AddOptions struct {
	Name             string
	Tag              string
	System           string
	To               string
	File             string
	Asset            string
	Source           bool
	All              bool
	GUI              bool
	Quiet            bool
	ChunkConcurrency int
	Target           string
}

func newAddCmd(handler CommandHandler) (*gcli.Command, func()) {
	opts := &AddOptions{ChunkConcurrency: -1}
	cmd := gcli.NewCommand("add", "Add a managed package")
	cmd.Config = func(c *gcli.Command) {
		c.StrOpt(&opts.Name, "name", "", "", "Managed package name")
		c.StrOpt(&opts.Tag, "tag", "", "", "Release tag")
		c.StrOpt(&opts.System, "system", "", "", "Target system")
		c.StrOpt(&opts.To, "to", "", "", "Install destination")
		c.StrOpt(&opts.File, "file", "", "", "File to extract")
		c.StrOpt(&opts.Asset, "asset", "", "", "Asset filter")
		c.BoolOpt(&opts.Source, "source", "", false, "Download source archive")
		c.BoolOpt(&opts.All, "extract-all", "ea", false, "Extract all files")
		c.BoolOpt(&opts.GUI, "gui", "", false, "Add as GUI application")
		c.BoolOpt(&opts.Quiet, "quiet", "", false, "Quiet output")
		c.IntOpt(&opts.ChunkConcurrency, "chunk", "", -1, "HTTP Range chunk concurrency: 0 auto, 1 single connection")
		c.AddArg("target", "Package target", true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Target = c.Arg("target").String()
		if err := validateNoFlagArgs(append([]string{opts.Target}, args...)); err != nil {
			return err
		}
		snapshot := *opts
		return handler("add", &snapshot)
	}
	return cmd, func() {
		*opts = AddOptions{ChunkConcurrency: -1}
	}
}
