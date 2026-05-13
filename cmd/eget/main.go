package main

import (
	"os"

	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/cli"
)

// Build-time variables injected via -ldflags
var (
	Version   = "dev"
	GitHash   = "unknown"
	BuildTime = "unknown"
)

func main() {
	cli.SetBuildInfo(Version, GitHash, BuildTime)

	if err := cli.Main(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		ccolor.Fprintf(os.Stderr, "❌ <red1>ERROR:</> <mga>%s</>\n", err)
		os.Exit(1)
	}
}
