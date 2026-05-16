package cli

import "github.com/gookit/gcli/v3"

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
	Targets          []string
}

func newInstallCmd(handler CommandHandler) (*gcli.Command, func()) {
	opts := &InstallOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
	cmd := gcli.NewCommand("install", "Install one or more targets")
	cmd.Aliases = []string{"i", "ins"}
	cmd.Config = func(c *gcli.Command) {
		c.StrOpt(&opts.Tag, "tag", "", "", "Release tag")
		c.StrOpt(&opts.System, "system", "", "", "Target system")
		c.StrOpt(&opts.To, "to", "", "", "Install destination")
		c.StrOpt(&opts.File, "file", "", "", "File to extract, multi use comma split, support glob")
		c.StrOpt(&opts.Asset, "asset", "a", "", "Asset filter, multi use comma split")
		c.StrOpt(&opts.Name, "name", "", "", "Managed package name when used with --add")
		c.BoolOpt(&opts.Source, "source", "", false, "Download source archive")
		c.BoolOpt(&opts.All, "extract-all", "ea", false, "Extract all files")
		c.BoolOpt(&opts.InstallAll, "all", "", false, "Install all managed packages from config")
		c.BoolOpt(&opts.GUI, "gui", "", false, "Install as GUI application")
		c.BoolOpt(&opts.Quiet, "quiet", "", false, "Quiet output")
		c.BoolOpt(&opts.Add, "add", "", false, "Add installed repo target to managed packages")
		c.IntOpt(&opts.FallbackVersions, "fallback-versions", "", 0, "Search older SourceForge version folders when asset is missing")
		c.IntOpt(&opts.ChunkConcurrency, "chunk", "", -1, "HTTP Range chunk concurrency: 0 auto, 1 single connection")
		c.IntOpt(&opts.BatchConcurrency, "batch", "", -1, "Concurrent package tasks for --all: 0 auto, 1 serial")
		c.AddArg("target", "Installation target(s)", false, true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		targetArgs := append(c.Arg("target").Strings(), args...)
		if err := validateNoFlagArgs(targetArgs); err != nil {
			return err
		}
		opts.Targets = splitTargets(targetArgs)
		snapshot := *opts
		snapshot.Targets = append([]string(nil), opts.Targets...)
		return handler("install", &snapshot)
	}
	return cmd, func() {
		*opts = InstallOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
	}
}
