package cli

import "github.com/gookit/goutil/cflag/capp"

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

func newAddCmd(handler CommandHandler) (*capp.Cmd, func()) {
	opts := &AddOptions{ChunkConcurrency: -1}
	cmd := capp.NewCmd("add", "Add a managed package", func(cmd *capp.Cmd) error {
		opts.Target = cmd.Arg("target").String()
		if err := validateNoTrailingFlags(cmd); err != nil {
			return err
		}
		snapshot := *opts
		return handler(cmd.Name, &snapshot)
	})

	cmd.StringVar(&opts.Name, "name", "", "Managed package name")
	cmd.StringVar(&opts.Tag, "tag", "", "Release tag")
	cmd.StringVar(&opts.System, "system", "", "Target system")
	cmd.StringVar(&opts.To, "to", "", "Install destination")
	cmd.StringVar(&opts.File, "file", "", "File to extract")
	cmd.StringVar(&opts.Asset, "asset", "", "Asset filter")
	cmd.BoolVar(&opts.Source, "source", false, "Download source archive")
	cmd.BoolVar(&opts.All, "extract-all", false, "Extract all files;;ea")
	cmd.BoolVar(&opts.GUI, "gui", false, "Add as GUI application")
	cmd.BoolVar(&opts.Quiet, "quiet", false, "Quiet output")
	cmd.IntVar(&opts.ChunkConcurrency, "chunk", -1, "HTTP Range chunk concurrency: 0 auto, 1 single connection")
	cmd.AddArg("target", "Package target", true, nil)
	return cmd, func() {
		*opts = AddOptions{ChunkConcurrency: -1}
	}
}
