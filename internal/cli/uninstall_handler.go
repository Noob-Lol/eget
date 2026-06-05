package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/inherelab/eget/internal/cli/prompts"
	"github.com/inherelab/eget/internal/install"
)

var uninstallRuntimeGOOS = runtime.GOOS

var openWindowsProgramsAndFeatures = func() error {
	return exec.Command("rundll32.exe", "shell32.dll,Control_RunDLL", "appwiz.cpl").Start()
}

func (s *cliService) handleUninstall(opts *UninstallOptions) error {
	if opts == nil || opts.Target == "" {
		return fmt.Errorf("remove target is required")
	}
	if !opts.Yes {
		confirmed, err := prompts.ConfirmRemove(opts.Target)
		if err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("remove cancelled")
		}
	}
	result, err := s.uninstallService.Uninstall(opts.Target)
	if err != nil {
		return err
	}
	fmt.Printf("repo: %s\n", result.Repo)
	if len(result.RemovedFiles) == 0 {
		fmt.Println("removed_files: 0")
		printManualUninstallHint(result.IsGUI, result.InstallMode)
		return nil
	}
	fmt.Printf("removed_files: %d\n", len(result.RemovedFiles))
	for _, file := range result.RemovedFiles {
		fmt.Printf("removed: %s\n", file)
	}
	printManualUninstallHint(result.IsGUI, result.InstallMode)
	return nil
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
