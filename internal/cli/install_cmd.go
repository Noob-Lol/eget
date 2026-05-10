package cli

import "github.com/gookit/goutil/cflag/capp"

type InstallOptions struct {
	Tag              string
	System           string
	To               string
	File             string
	Asset            string
	Name             string
	Source           bool
	All              bool
	InstallAll       bool
	GUI              bool
	Quiet            bool
	Add              bool
	FallbackVersions int
	ChunkConcurrency int
	BatchConcurrency int
	Target           string
}

func newInstallCmd(handler CommandHandler) (*capp.Cmd, func()) {
	opts := &InstallOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
	cmd := capp.NewCmd("install", "Install a target", func(cmd *capp.Cmd) error {
		opts.Target = cmd.Arg("target").String()
		if err := validateNoTrailingFlags(cmd); err != nil {
			return err
		}
		snapshot := *opts
		return handler(cmd.Name, &snapshot)
	})
	cmd.Aliases = []string{"i", "ins"}

	cmd.StringVar(&opts.Tag, "tag", "", "Release tag")
	cmd.StringVar(&opts.System, "system", "", "Target system")
	cmd.StringVar(&opts.To, "to", "", "Install destination")
	cmd.StringVar(&opts.File, "file", "", "File to extract, multi use comma split, support glob")
	cmd.StringVar(&opts.Asset, "asset", "", "Asset filter, multi use comma split;;a")
	cmd.StringVar(&opts.Name, "name", "", "Managed package name when used with --add")
	cmd.BoolVar(&opts.Source, "source", false, "Download source archive")
	cmd.BoolVar(&opts.All, "extract-all", false, "Extract all files;;ea")
	cmd.BoolVar(&opts.InstallAll, "all", false, "Install all managed packages from config")
	cmd.BoolVar(&opts.GUI, "gui", false, "Install as GUI application")
	cmd.BoolVar(&opts.Quiet, "quiet", false, "Quiet output")
	cmd.BoolVar(&opts.Add, "add", false, "Add installed repo target to managed packages")
	cmd.IntVar(&opts.FallbackVersions, "fallback-versions", 0, "Search older SourceForge version folders when asset is missing")
	cmd.IntVar(&opts.ChunkConcurrency, "chunk", -1, "HTTP Range chunk concurrency: 0 auto, 1 single connection")
	cmd.IntVar(&opts.BatchConcurrency, "batch", -1, "Concurrent package tasks for --all: 0 auto, 1 serial")
	cmd.AddArg("target", "Installation target", false, nil)
	return cmd, func() {
		*opts = InstallOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
	}
}
