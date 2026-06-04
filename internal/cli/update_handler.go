package cli

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/gookit/goutil/cliutil"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/app"
)

func (s *cliService) handleUpdate(opts *UpdateOptions) error {
	if opts.Self {
		if opts.All {
			return fmt.Errorf("update --self cannot be used with --all")
		}
		if len(opts.Targets) > 0 {
			return fmt.Errorf("update --self cannot be used with target")
		}
		if opts.BatchConcurrency > 0 {
			return fmt.Errorf("--batch can only be used with --all")
		}
		if s.selfUpdateService == nil {
			return fmt.Errorf("self update service is required")
		}
		source := s.selfUpdateSource(opts)
		if opts.Check {
			printSelfUpdateCheckSource(source)
		}
		result, err := s.selfUpdateService.Update(app.SelfUpdateOptions{
			CheckOnly: opts.Check,
			Tag:       opts.Tag,
			Source:    source,
			Asset:     splitAssetFilters(opts.Asset),
			Install:   s.applyGlobalFlags(installOptionsFromUpdate(opts)),
		})
		if err != nil {
			return err
		}
		printSelfUpdateResult(result)
		return nil
	}
	if opts.Check {
		if len(opts.Targets) > 0 {
			return s.handleUpdateCheckTargets(opts.Targets)
		}
		return s.handleList(&ListOptions{Outdated: true})
	}
	if opts.DryRun {
		return fmt.Errorf("update --dry-run is not implemented")
	}
	if opts.Interactive {
		return fmt.Errorf("update --interactive is not implemented")
	}
	installOpts := s.applyGlobalFlags(installOptionsFromUpdate(opts))
	if opts.All {
		ccolor.Infoln("🚀 Checking outdated packages")
		s.printOutdatedProxyNotice()
		reporter := newOutdatedProgressReporter(s.stderrWriter(), !appVerbose())
		cacheNotices := &apiCacheNoticeCounter{}
		restoreNotices := suppressOutdatedNetworkNotices(cacheNotices)
		defer restoreNotices()
		prevOnDone := s.updService.OnCheckDone
		s.updService.OnCheckDone = reporter.OnCheckDone
		defer func() {
			s.updService.OnCheckDone = prevOnDone
		}()

		items, failures, checked, err := s.updService.ListUpdateCandidates()
		reporter.Finish()
		if err != nil {
			return err
		}
		s.printAPICacheSummary(cacheNotices.Count())
		ccolor.Successf("✅ Checked %d packages\n", checked)

		for _, failure := range failures {
			ccolor.Fprintf(os.Stderr, "<yellow>check_failed</> %s (%s): %v\n", failure.Name, failure.Repo, failure.Error)
		}
		if len(items) == 0 {
			ccolor.Cyanln("🎉 No outdated packages found")
			return nil
		}

		cols := []string{"Name", "Repo", "Installed", "Latest version"}
		rows := make([][]any, 0, len(items))
		for _, item := range items {
			rows = append(rows, []any{item.Name, item.Repo, item.InstalledTag, item.LatestTag})
		}
		ccolor.Print(cliutil.FormatTable(cols, rows, cliutil.MinimalStyle))

		ccolor.Magentaf("\n🪄🚀 Updating %d packages:\n", len(items))
		_, err = s.updService.UpdateCandidates(items, installOpts)
		return err
	}
	if opts.BatchConcurrency > 0 {
		return fmt.Errorf("--batch can only be used with --all")
	}
	if len(opts.Targets) == 0 {
		return fmt.Errorf("update target is required")
	}
	var failures []error
	for _, target := range opts.Targets {
		if _, err := s.updService.UpdatePackage(target, installOpts); err != nil {
			ccolor.Fprintf(s.stderrWriter(), "<yellow>update_failed</> %s: %v\n", target, err)
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d update failed", len(failures))
	}
	return nil
}

func (s *cliService) handleUpdateCheckTargets(targets []string) error {
	ccolor.Infoln("🚀 Checking outdated packages")
	s.printOutdatedProxyNotice()
	reporter := newOutdatedProgressReporter(s.stderrWriter(), !appVerbose())
	cacheNotices := &apiCacheNoticeCounter{}
	restoreNotices := suppressOutdatedNetworkNotices(cacheNotices)
	defer restoreNotices()
	prevOnDone := s.updService.OnCheckDone
	s.updService.OnCheckDone = reporter.OnCheckDone
	defer func() {
		s.updService.OnCheckDone = prevOnDone
	}()

	items, failures, checked, err := s.updService.ListUpdateCandidatesForTargets(targets)
	reporter.Finish()
	if err != nil {
		return err
	}
	s.printAPICacheSummary(cacheNotices.Count())
	ccolor.Successf("✅ Checked %d packages\n", checked)

	for _, failure := range failures {
		ccolor.Fprintf(os.Stderr, "<yellow>check_failed</> %s (%s): %v\n", failure.Name, failure.Repo, failure.Error)
	}
	if len(items) == 0 {
		ccolor.Cyanln("🎉 No outdated packages found")
		return nil
	}

	ccolor.Infoln("Outdated Packages:")
	cols := []string{"Name", "Repo", "Version", "Latest version", "Published At"}
	rows := make([][]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, []any{item.Name, item.Repo, item.InstalledTag, item.LatestTag, formatOutdatedTime(item.PublishedAt)})
	}
	ccolor.Print(cliutil.FormatTable(cols, rows, cliutil.MinimalStyle))
	return nil
}

func (s *cliService) selfUpdateSource(opts *UpdateOptions) string {
	if opts != nil && opts.SelfSource != "" {
		return opts.SelfSource
	}
	lookupEnv := os.LookupEnv
	if s != nil && s.lookupEnv != nil {
		lookupEnv = s.lookupEnv
	}
	if value, ok := lookupEnv("EGET_SELF_UPDATE_SOURCE"); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func printSelfUpdateCheckSource(source string) {
	if strings.TrimSpace(source) == "" {
		ccolor.Infof("Checking self update from: github.com (%s)\n", app.SelfUpdateRepo)
		return
	}
	host := hostFromURL(source)
	if host == "" {
		host = source
	}
	ccolor.Infof("Checking self update from: %s (%s)\n", host, source)
}

func hostFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func printSelfUpdateResult(result app.SelfUpdateResult) {
	if !result.Outdated && !result.Updated {
		ccolor.Cyanf("eget is already up to date: %s\n", result.CurrentVersion)
		return
	}
	if result.Updated && result.Deferred {
		ccolor.Successf("eget update downloaded. It will be replaced after this process exits: %s\n", result.LatestVersion)
		return
	}
	if result.Updated {
		ccolor.Successf("eget updated: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
		return
	}
	ccolor.Infof("eget update available: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
}
