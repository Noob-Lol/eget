package cli

import "github.com/gookit/goutil/cflag/capp"

type UpdateOptions struct {
	All              bool
	Check            bool
	DryRun           bool
	Interactive      bool
	Tag              string
	System           string
	To               string
	File             string
	Asset            string
	Source           bool
	Quiet            bool
	ChunkConcurrency int
	BatchConcurrency int
	Targets          []string
}

func newUpdateCmd(handler CommandHandler) (*capp.Cmd, func()) {
	opts := &UpdateOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
	cmd := capp.NewCmd("update", "Update installed targets", func(cmd *capp.Cmd) error {
		targetArgs := cmd.Arg("target").Strings()
		if err := validateNoFlagArgs(targetArgs); err != nil {
			return err
		}
		opts.Targets = splitTargets(targetArgs)
		if err := validateNoTrailingFlags(cmd); err != nil {
			return err
		}
		snapshot := *opts
		snapshot.Targets = append([]string(nil), opts.Targets...)
		return handler(cmd.Name, &snapshot)
	})
	cmd.Aliases = []string{"up"}

	cmd.BoolVar(&opts.All, "all", false, "Update all managed packages;;A")
	cmd.BoolVar(&opts.Check, "check", false, "Check and list outdated installed packages")
	cmd.BoolVar(&opts.DryRun, "dry-run", false, "Preview updates without changes")
	cmd.BoolVar(&opts.Interactive, "interactive", false, "Interactively choose packages")
	cmd.StringVar(&opts.Tag, "tag", "", "Release tag")
	cmd.StringVar(&opts.System, "system", "", "Target system")
	cmd.StringVar(&opts.To, "to", "", "Install destination")
	cmd.StringVar(&opts.File, "file", "", "File to extract, multi use comma split, support glob")
	cmd.StringVar(&opts.Asset, "asset", "", "Asset filter, multi use comma split;;a")
	cmd.BoolVar(&opts.Source, "source", false, "Download source archive")
	cmd.BoolVar(&opts.Quiet, "quiet", false, "Quiet output")
	cmd.IntVar(&opts.ChunkConcurrency, "chunk", -1, "HTTP Range chunk concurrency: 0 auto, 1 single connection")
	cmd.IntVar(&opts.BatchConcurrency, "batch", -1, "Concurrent package tasks for --all: 0 auto, 1 serial")
	cmd.AddArg("target", "Target(s) to update", false, nil, true)
	return cmd, func() {
		*opts = UpdateOptions{ChunkConcurrency: -1, BatchConcurrency: -1}
	}
}
