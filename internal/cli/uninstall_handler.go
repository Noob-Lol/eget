package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/inherelab/eget/internal/app"
	"github.com/inherelab/eget/internal/cli/prompts"
	"github.com/inherelab/eget/internal/install"
)

var uninstallRuntimeGOOS = runtime.GOOS

var openWindowsProgramsAndFeatures = func() error {
	return exec.Command("rundll32.exe", "shell32.dll,Control_RunDLL", "appwiz.cpl").Start()
}

func (s *cliService) handleUninstall(opts *UninstallOptions) error {
	if opts == nil {
		return fmt.Errorf("remove target is required")
	}
	targets := opts.Targets
	if len(targets) == 0 && opts.Target != "" {
		targets = []string{opts.Target}
	}
	if len(targets) == 0 {
		return fmt.Errorf("remove target is required")
	}
	for _, target := range targets {
		if err := s.uninstallOne(target, opts); err != nil {
			return err
		}
	}
	return nil
}

func (s *cliService) uninstallOne(target string, opts *UninstallOptions) error {
	if !opts.Yes {
		confirmed, err := prompts.ConfirmRemove(target)
		if err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("remove cancelled")
		}
	}
	result, err := s.uninstallService.UninstallWithOptions(target, app.UninstallOptions{Purge: opts.Purge})
	if err != nil {
		return err
	}
	fmt.Printf("repo: %s\n", result.Repo)
	if len(result.RemovedFiles) == 0 {
		fmt.Println("removed_files: 0")
		printPurgedConfig(result.PurgedConfig)
		printManualUninstallHint(result.IsGUI, result.InstallMode)
		return nil
	}
	fmt.Printf("removed_files: %d\n", len(result.RemovedFiles))
	for _, file := range result.RemovedFiles {
		fmt.Printf("removed: %s\n", file)
	}
	printPurgedConfig(result.PurgedConfig)
	printManualUninstallHint(result.IsGUI, result.InstallMode)
	return nil
}

func printPurgedConfig(name string) {
	if name != "" {
		fmt.Printf("purged_config: %s\n", name)
	}
}

func printManualUninstallHint(isGUI bool, installMode string) {
	if !isGUI || installMode != install.InstallModeInstaller {
		return
	}
	fmt.Println("manual_uninstall_required: this package was installed by a GUI installer; remove it from the system apps/uninstall settings.")
	if uninstallRuntimeGOOS != "windows" {
		return
	}
	if err := openWindowsProgramsAndFeatures(); err != nil {
		fmt.Printf("warning: failed to open Windows Programs and Features: %v\n", err)
		return
	}
	fmt.Println("opened_uninstall_settings: Windows Programs and Features")
}
