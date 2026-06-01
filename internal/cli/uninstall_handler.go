package cli

import "fmt"

func (s *cliService) handleUninstall(opts *UninstallOptions) error {
	if opts == nil || opts.Target == "" {
		return fmt.Errorf("remove target is required")
	}
	if !opts.Yes {
		confirmed, err := promptConfirmRemove(opts.Target)
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
		return nil
	}
	fmt.Printf("removed_files: %d\n", len(result.RemovedFiles))
	for _, file := range result.RemovedFiles {
		fmt.Printf("removed: %s\n", file)
	}
	return nil
}
