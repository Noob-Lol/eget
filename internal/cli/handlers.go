package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gookit/cliui/progress"
	"github.com/gookit/cliui/show"
	"github.com/gookit/cliui/show/lists"
	"github.com/gookit/goutil/cliutil"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/app"
	"github.com/inherelab/eget/internal/client"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/sdk"
)

func (s *cliService) handle(name string, options any) error {
	switch name {
	case "install":
		opts := options.(*InstallOptions)
		cliInstallOpts := installOptionsFromInstall(opts)
		if opts.InstallAll {
			if len(opts.Targets) > 0 {
				return fmt.Errorf("install --all cannot be used with target")
			}
			if opts.Add {
				return fmt.Errorf("install --all cannot be used with --add")
			}
			s.warnIfSudoUserConfigLooksSkipped(opts.Quiet)
			_, err := s.appService.InstallAllPackages(cliInstallOpts)
			return err
		}
		if opts.BatchConcurrency > 0 {
			return fmt.Errorf("--batch can only be used with --all")
		}
		if len(opts.Targets) == 0 {
			return fmt.Errorf("install target is required")
		}
		if len(opts.Targets) > 1 && opts.Name != "" {
			return fmt.Errorf("install --name cannot be used with multiple targets")
		}
		s.warnIfSudoUserConfigLooksSkipped(opts.Quiet)
		for _, target := range opts.Targets {
			_, err := s.appService.InstallTarget(target, cliInstallOpts, app.InstallExtras{
				AddToConfig: opts.Add,
				PackageName: opts.Name,
				PackageOpts: cliInstallOpts,
			})
			if err != nil {
				return err
			}
			if opts.Add {
				pkgName := opts.Name
				if pkgName == "" {
					if repo, repoErr := install.NormalizeRepoTarget(target); repoErr == nil {
						if _, name, found := strings.Cut(repo, "/"); found {
							pkgName = name
						}
					}
				}
				if pkgName != "" {
					fmt.Printf("Added package config: %s -> %s\n", pkgName, target)
				}
			}
		}
		return nil
	case "download":
		opts := options.(*DownloadOptions)
		_, err := s.appService.DownloadTarget(opts.Target, installOptionsFromDownload(opts))
		return err
	case "add":
		opts := options.(*AddOptions)
		err := s.cfgService.AddPackage(opts.Target, opts.Name, installOptionsFromAdd(opts))
		if err == nil {
			pkgName := opts.Name
			if pkgName == "" {
				if inferred, inferErr := app.ResolvePackageName(opts.Target, opts.Name); inferErr == nil {
					pkgName = inferred
				}
			}
			ccolor.Infof("✓ Added package config: %s -> %s\n", pkgName, opts.Target)
		}
		return err
	case "uninstall":
		opts := options.(*UninstallOptions)
		return s.handleUninstall(opts)
	case "list":
		opts := options.(*ListOptions)
		return s.handleList(opts)
	case "show":
		opts := options.(*ShowOptions)
		return s.handleShow(opts)
	case "config":
		opts := options.(*ConfigOptions)
		return s.handleConfig(opts)
	case "query":
		opts := options.(*QueryOptions)
		return s.handleQuery(opts)
	case "search":
		opts := options.(*SearchOptions)
		return s.handleSearch(opts)
	case "update":
		opts := options.(*UpdateOptions)
		return s.handleUpdate(opts)
	case "sdk.install":
		opts := options.(*SDKInstallOptions)
		return s.handleSDKInstall(opts)
	case "sdk.list":
		opts := options.(*SDKListOptions)
		return s.handleSDKList(opts)
	case "sdk.remove":
		opts := options.(*SDKRemoveOptions)
		return s.handleSDKRemove(opts)
	case "sdk.search":
		opts := options.(*SDKSearchOptions)
		return s.handleSDKSearch(opts)
	case "sdk.index.list", "sdk.index.show", "sdk.index.refresh", "sdk.index.clear":
		opts := options.(*SDKIndexOptions)
		return s.handleSDKIndex(opts)
	case "sdk.config.add":
		opts := options.(*SDKConfigOptions)
		return s.handleSDKConfig(opts)
	default:
		return ErrNotImplemented
	}
}

func (s *cliService) warnIfSudoUserConfigLooksSkipped(quiet bool) {
	if quiet {
		return
	}

	lookupEnv := os.LookupEnv
	if s.lookupEnv != nil {
		lookupEnv = s.lookupEnv
	}
	if configPath, ok := lookupEnv("EGET_CONFIG"); ok && configPath != "" {
		return
	}

	sudoUser, ok := lookupEnv("SUDO_USER")
	if !ok || sudoUser == "" || sudoUser == "root" {
		return
	}

	resolveConfigPath := cfgpkg.ResolveConfigPath
	if s.configPathResolver != nil {
		resolveConfigPath = s.configPathResolver
	}
	if _, err := resolveConfigPath(); err == nil {
		return
	} else if !cfgpkg.IsNotExist(err) && !errors.Is(err, os.ErrNotExist) {
		return
	}

	lookupHome := lookupUserHome
	if s.lookupUserHome != nil {
		lookupHome = s.lookupUserHome
	}
	homeDir, err := lookupHome(sudoUser)
	if err != nil || homeDir == "" {
		return
	}

	candidate := cfgpkg.OSConfigPath(homeDir, "linux", lookupEnv)
	exists := fileExists
	if s.fileExists != nil {
		exists = s.fileExists
	}
	if !exists(candidate) {
		return
	}

	displayPath := filepath.ToSlash(candidate)
	ccolor.Fprintf(
		s.stderrWriter(),
		"<yellow>Warning</>: sudo may be using a different HOME, so eget did not load %s. Try: sudo EGET_CONFIG=%q eget install ...\n",
		displayPath,
		displayPath,
	)
}

func lookupUserHome(name string) (string, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && !info.IsDir()
}

func (s *cliService) handleUninstall(opts *UninstallOptions) error {
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

func (s *cliService) handleList(opts *ListOptions) error {
	if opts != nil && opts.Outdated && opts.Info != "" {
		return fmt.Errorf("list --outdated and --info cannot be used together")
	}
	if opts != nil && opts.Outdated && opts.NoInstalled {
		return fmt.Errorf("list --outdated and --no-installed cannot be used together")
	}
	if opts != nil && opts.NoInstalled && opts.Info != "" {
		return fmt.Errorf("list --no-installed and --info cannot be used together")
	}
	if opts != nil && opts.Info != "" {
		item, err := s.listService.FindPackage(opts.Info)
		if err != nil {
			return err
		}
		show.AList("Package Info", listItemToDisplay(*item))
		return nil
	}

	if opts != nil && opts.Outdated {
		ccolor.Infoln("🚀 Checking outdated packages")
		s.printOutdatedProxyNotice()
		reporter := newOutdatedProgressReporter(s.stderrWriter(), !appVerbose())
		cacheNotices := &apiCacheNoticeCounter{}
		restoreNotices := suppressOutdatedNetworkNotices(cacheNotices)
		defer restoreNotices()
		prevOnDone := s.listService.OnCheckDone
		s.listService.OnCheckDone = reporter.OnCheckDone
		defer func() {
			s.listService.OnCheckDone = prevOnDone
		}()
		items, failures, checked, err := s.listService.ListOutdatedPackages()
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

	var items []app.ListItem
	var err error
	if opts != nil && opts.NoInstalled {
		items, err = s.listService.ListPackages()
		items = filterNoInstalledListItems(items)
		if opts.GUI {
			items = filterGUIListItems(items)
		}
	} else if opts != nil && opts.GUI {
		items, err = s.listService.ListGUIPackages(opts.All)
	} else if opts != nil && opts.All {
		items, err = s.listService.ListPackages()
	} else {
		items, err = s.listService.ListInstalledPackages()
	}
	if err != nil {
		return err
	}
	if len(items) == 0 {
		if opts != nil && opts.GUI {
			ccolor.Infoln("no GUI packages found")
		} else if opts != nil && opts.All {
			ccolor.Infoln("no managed packages found")
		} else {
			ccolor.Infoln("no installed packages found")
		}
		return nil
	}

	switch {
	case opts != nil && opts.NoInstalled:
		ccolor.Infoln("Not Installed Packages:")
	case opts != nil && opts.All:
		ccolor.Infoln("Managed Packages:")
	case opts == nil || !opts.GUI:
		ccolor.Infoln("Installed Packages:")
	}
	cols := []string{"Name", "Repo", "Version", "Source", "Update Time"}
	rows := make([][]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, listPackageRow(item))
	}
	ccolor.Print(cliutil.FormatTable(cols, rows, cliutil.MinimalStyle))
	return nil
}

func (s *cliService) handleShow(opts *ShowOptions) error {
	if opts == nil || opts.Target == "" {
		return fmt.Errorf("show target is required")
	}
	result, err := s.showService.ShowPackage(opts.Target)
	if err != nil {
		return err
	}
	show.AList("Package Details", showResultToDisplay(result))
	return nil
}

func filterNoInstalledListItems(items []app.ListItem) []app.ListItem {
	filtered := make([]app.ListItem, 0, len(items))
	for _, item := range items {
		if !item.Installed {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterGUIListItems(items []app.ListItem) []app.ListItem {
	filtered := make([]app.ListItem, 0, len(items))
	for _, item := range items {
		if item.IsGUI {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func listPackageRow(item app.ListItem) []any {
	return []any{
		item.Name,
		item.Repo,
		listPackageVersion(item),
		packageSource(item),
		formatListTime(item.InstalledAt),
	}
}

func listPackageVersion(item app.ListItem) string {
	if item.Version != "" {
		return item.Version
	}
	if !item.Installed {
		return "No-Install"
	}
	return ""
}

func packageSource(item app.ListItem) string {
	switch install.DetectTargetKind(item.Repo) {
	case install.TargetRepo, install.TargetGitHubURL:
		return "github"
	case install.TargetSourceForge:
		return "sourceforge"
	case install.TargetForge:
		return "forge"
	case install.TargetTemplate:
		return "template"
	case install.TargetDirectURL:
		return "url"
	case install.TargetLocalFile:
		return "local"
	default:
		if item.Repo == "" {
			return "-"
		}
		return "unknown"
	}
}

func formatListTime(value time.Time) string {
	return compactTime(value)
}

func formatOutdatedTime(value time.Time) string {
	return formatListTime(value)
}

func appVerbose() bool {
	return cliApp != nil && cliApp.Verbose()
}

type outdatedProgressReporter struct {
	writer  io.Writer
	bar     *progress.Progress
	enabled bool
	started bool
}

func newOutdatedProgressReporter(writer io.Writer, enabled bool) *outdatedProgressReporter {
	if writer == nil {
		writer = os.Stderr
	}
	return &outdatedProgressReporter{
		writer:  writer,
		enabled: enabled && progress.IsTerminal(writer),
	}
}

func (r *outdatedProgressReporter) OnCheckDone(checked, total int) {
	if r == nil || !r.enabled || total <= 0 {
		return
	}
	if r.bar == nil {
		r.bar = progress.Bar(int64(total))
		r.bar.Out = r.writer
		r.bar.Format = "Checking ({@current}/{@max}) [{@bar}]"
		r.bar.Start()
		r.started = true
	}
	r.bar.AdvanceTo(int64(checked))
}

func (r *outdatedProgressReporter) Finish() {
	if r == nil || !r.started || r.bar == nil {
		return
	}
	r.bar.Finish()
}

type apiCacheNoticeCounter struct {
	mu    sync.Mutex
	count int
}

func (c *apiCacheNoticeCounter) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
	return len(p), nil
}

func (c *apiCacheNoticeCounter) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func suppressOutdatedNetworkNotices(cacheNotices io.Writer) func() {
	prevInstallProxy := install.SetProxyNoticeWriter(io.Discard)
	prevInstallCache := install.SetAPICacheNoticeWriter(cacheNotices)
	prevClientProxy := client.SetProxyNoticeWriter(io.Discard)
	prevClientCache := client.SetAPICacheNoticeWriter(cacheNotices)
	return func() {
		install.SetProxyNoticeWriter(prevInstallProxy)
		install.SetAPICacheNoticeWriter(prevInstallCache)
		client.SetProxyNoticeWriter(prevClientProxy)
		client.SetAPICacheNoticeWriter(prevClientCache)
	}
}

func (s *cliService) printOutdatedProxyNotice() {
	if s == nil || s.proxyURL == "" {
		return
	}
	ccolor.Fprintf(s.stderrWriter(), " - Using <ylw>proxy_url for GitHub API request</>: %s\n", s.proxyURL)
}

func (s *cliService) printAPICacheSummary(count int) {
	if count <= 0 {
		return
	}
	ccolor.Fprintf(s.stderrWriter(), " - Reused <ylw>api_cache files</>: %d\n", count)
}

func (s *cliService) handleConfig(opts *ConfigOptions) error {
	switch opts.Action {
	case "init":
		info, err := s.cfgService.ConfigInfo()
		if err != nil {
			return err
		}
		if info.Exists {
			confirmed, err := promptConfirmOverwrite(info.Path)
			if err != nil {
				return err
			}
			if !confirmed {
				return fmt.Errorf("config init cancelled")
			}
		}
		path, err := s.cfgService.ConfigInit()
		if err != nil {
			return err
		}
		ccolor.Successf("✓ Initialized config: %s\n", path)
		return nil
	case "list", "ls":
		info, err := s.cfgService.ConfigInfo()
		if err != nil {
			return err
		}
		cfg, err := s.cfgService.ConfigList()
		if err != nil {
			return err
		}

		showListConfig := func(opts *show.ListOptions) {
			opts.TagName = "toml"
			opts.FilterFunc = func(item *lists.Item) bool {
				switch item.RftVal().Kind() {
				case reflect.Map, reflect.Slice:
					if item.RftVal().IsNil() || item.RftVal().Len() == 0 {
						return false
					}
				}
				return true
			}
		}

		ccolor.Printf("# %s, exists: %v\n", info.Path, info.Exists)
		show.MList(map[string]any{
			"global":   cfg.Global,
			"apiCache": cfg.ApiCache,
			"ghproxy":  cfg.Ghproxy,
		}, showListConfig)

		// packages
		ccolor.Grayln("---------------------------")
		ccolor.Yellowln("📦 Configed Packages:")
		show.MList(cfg.Packages, showListConfig)

		// sdk
		ccolor.Grayln("---------------------------")
		ccolor.Yellowln("📦 Configed SDKs:")
		show.MList(cfg.SDK, showListConfig)
		return nil
	case "get":
		value, err := s.cfgService.ConfigGet(opts.Key)
		if err != nil {
			return err
		}
		if value == nil {
			ccolor.Infoln("nil")
		} else if str, ok := value.(string); ok {
			ccolor.Infoln(str)
		} else {
			show.JSON(value)
		}
		return nil
	case "set":
		err := s.cfgService.ConfigSet(opts.Key, opts.Value)
		if err == nil {
			ccolor.Successf("✓ Set config: %s = %s\n", opts.Key, opts.Value)
		}
		return err
	default:
		return fmt.Errorf("config action is required")
	}
}

func (s *cliService) handleUpdate(opts *UpdateOptions) error {
	if opts.Check {
		return s.handleList(&ListOptions{Outdated: true})
	}
	if opts.DryRun {
		return fmt.Errorf("update --dry-run is not implemented")
	}
	if opts.Interactive {
		return fmt.Errorf("update --interactive is not implemented")
	}
	installOpts := installOptionsFromUpdate(opts)
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
	for _, target := range opts.Targets {
		if _, err := s.updService.UpdatePackage(target, installOpts); err != nil {
			return err
		}
	}
	return nil
}

func (s *cliService) handleSDKInstall(opts *SDKInstallOptions) error {
	if opts == nil || len(opts.Targets) == 0 {
		return fmt.Errorf("sdk install target is required")
	}
	results, err := s.sdkService.InstallMany(context.Background(), opts.Targets, sdk.InstallOptions{
		Force:    opts.Force,
		Progress: s.sdkDownloadProgress(),
		OnStart: func(target string, version string, host string) {
			ccolor.Printf(" - Install SDK %s -> %s from %s\n", target, version, host)
		},
	})
	if err != nil {
		return err
	}
	for _, result := range results {
		notes := sdkResultNotes(result.Cached, result.Resumed)
		if notes != "" {
			ccolor.Successf("✓ Installed %s@%s -> %s (%s)\n", result.Name, result.Version, result.Path, notes)
			continue
		}
		ccolor.Successf("✓ Installed %s@%s -> %s\n", result.Name, result.Version, result.Path)
	}
	return nil
}

func (s *cliService) sdkDownloadProgress() func(int64) io.Writer {
	return func(size int64) io.Writer {
		return install.NewDownloadProgress(s.stderrWriter(), size)
	}
}

func (s *cliService) handleSDKList(opts *SDKListOptions) error {
	name := ""
	jsonOutput := false
	if opts != nil {
		name = opts.Name
		jsonOutput = opts.JSON
	}
	entries, err := s.sdkService.List(name)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(sdkEntriesToDisplay(entries))
	}
	if len(entries) == 0 {
		ccolor.Infoln("no SDK versions installed")
		return nil
	}
	cols := []string{"Name", "Version", "Path", "Installed At"}
	rows := make([][]any, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []any{entry.Name, entry.Version, entry.Path, compactTime(entry.InstalledAt)})
	}
	ccolor.Print(cliutil.FormatTable(cols, rows, cliutil.MinimalStyle))
	return nil
}

func (s *cliService) handleSDKRemove(opts *SDKRemoveOptions) error {
	if opts == nil || opts.Target == "" {
		return fmt.Errorf("sdk remove target is required")
	}
	result, err := s.sdkService.Remove(opts.Target)
	if err != nil {
		return err
	}
	if result.Missing {
		ccolor.Warnf("SDK directory already missing: %s\n", result.Path)
		return nil
	}
	ccolor.Successf("✓ Removed %s@%s -> %s\n", result.Name, result.Version, result.Path)
	return nil
}

func (s *cliService) handleSDKSearch(opts *SDKSearchOptions) error {
	if opts == nil || opts.Name == "" {
		return fmt.Errorf("sdk search name is required")
	}
	results, err := s.sdkService.SearchIndex(opts.Name, sdk.SearchOptions{
		Keywords: opts.Keywords,
		Number:   opts.Number,
		Sort:     opts.Sort,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		return printJSON(results)
	}
	if len(results) == 0 {
		ccolor.Infoln("no SDK index matches found")
		return nil
	}
	cols := []string{"Version", "Stable", "OS", "Arch", "Ext", "Filename"}
	rows := make([][]any, 0, len(results))
	for _, result := range results {
		rows = append(rows, []any{result.Version, result.Stable, result.OS, result.Arch, result.Ext, result.Filename})
	}
	ccolor.Print(cliutil.FormatTable(cols, rows, cliutil.MinimalStyle))
	return nil
}

func (s *cliService) handleSDKIndex(opts *SDKIndexOptions) error {
	if opts == nil {
		return fmt.Errorf("sdk index action is required")
	}
	switch opts.Action {
	case "list":
		infos, err := s.sdkService.ListIndexes()
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSON(sdkCachedIndexesToDisplay(infos))
		}
		if len(infos) == 0 {
			ccolor.Infoln("no SDK index cache found")
			return nil
		}
		cols := []string{"Name", "Versions", "Source", "Updated At"}
		rows := make([][]any, 0, len(infos))
		for _, info := range infos {
			versions := any(info.Versions)
			updatedAt := compactTime(info.FetchedAt)
			if !info.Cached {
				versions = "-"
				updatedAt = "-"
			}
			rows = append(rows, []any{info.SDK, versions, info.SourceURL, updatedAt})
		}
		ccolor.Print(cliutil.FormatTable(cols, rows, cliutil.MinimalStyle))
		return nil
	case "show":
		index, err := s.sdkService.ShowIndex(opts.Name)
		if err != nil {
			return err
		}
		printSDKIndexSummary(index)
		return nil
	case "refresh":
		svc := s.sdkServiceWithIndexReporter()
		if opts.All {
			indexes, err := svc.RefreshAllIndexes(context.Background())
			if err != nil {
				return err
			}
			for _, index := range indexes {
				ccolor.Successf("✓ Refreshed SDK index: %s (%d versions)\n", index.SDK, len(index.Items))
			}
			return nil
		}
		index, err := svc.RefreshIndex(context.Background(), opts.Name)
		if err != nil {
			return err
		}
		ccolor.Successf("✓ Refreshed SDK index: %s (%d versions)\n", index.SDK, len(index.Items))
		return nil
	case "clear":
		if opts.All {
			if err := s.sdkService.ClearAllIndexes(); err != nil {
				return err
			}
			ccolor.Successln("✓ Cleared all SDK indexes")
			return nil
		}
		if err := s.sdkService.ClearIndex(opts.Name); err != nil {
			return err
		}
		ccolor.Successf("✓ Cleared SDK index: %s\n", opts.Name)
		return nil
	default:
		return fmt.Errorf("sdk index action is required")
	}
}

func (s *cliService) handleSDKConfig(opts *SDKConfigOptions) error {
	if opts == nil || opts.Action != "add" {
		return fmt.Errorf("sdk config action is required")
	}
	result, err := s.cfgService.AddSDKConfig(app.SDKConfigAddOptions{
		Name:   opts.Name,
		All:    opts.All,
		Force:  opts.Force,
		Mirror: opts.Mirror,
	})
	if err != nil {
		return err
	}
	for _, item := range result.Items {
		source := item.Source
		if source == "" {
			source = "official"
		}
		switch item.Action {
		case app.SDKConfigActionAdded:
			ccolor.Successf("✓ Added SDK config: %s (%s)\n", item.Name, source)
		case app.SDKConfigActionUpdated:
			ccolor.Successf("✓ Updated SDK config: %s (%s)\n", item.Name, source)
		case app.SDKConfigActionSkipped:
			ccolor.Infof("- Skipped SDK config: %s %s\n", item.Name, item.Reason)
		}
	}
	return nil
}

func (s *cliService) sdkServiceWithIndexReporter() sdkCLIService {
	reporter := s.sdkIndexRefreshReporter()
	switch svc := s.sdkService.(type) {
	case sdk.Service:
		svc.OnIndexRefresh = reporter
		return svc
	case *sdk.Service:
		cloned := *svc
		cloned.OnIndexRefresh = reporter
		return &cloned
	default:
		return s.sdkService
	}
}

func (s *cliService) sdkIndexRefreshReporter() func(sdk.IndexRefreshEvent) {
	return func(event sdk.IndexRefreshEvent) {
		switch event.Stage {
		case sdk.IndexRefreshFetchStart:
			ccolor.Fprintf(s.stderrWriter(), " - Fetch SDK index %s: %s\n", event.SDK, event.URL)
		case sdk.IndexRefreshFetchDone:
			ccolor.Fprintf(s.stderrWriter(), " - Fetched SDK index %s\n", event.SDK)
		case sdk.IndexRefreshParseStart:
			parser := event.Parser
			if parser == "" {
				parser = "-"
			}
			ccolor.Fprintf(s.stderrWriter(), " - Parse SDK index %s: format=%s parser=%s\n", event.SDK, event.Format, parser)
		case sdk.IndexRefreshParseDone:
			ccolor.Fprintf(s.stderrWriter(), " - Parsed SDK index %s: %d versions, %d files\n", event.SDK, event.Versions, event.Files)
		case sdk.IndexRefreshParseFailed:
			ccolor.Fprintf(s.stderrWriter(), " - Parse SDK index failed %s: %v\n", event.SDK, event.Err)
		case sdk.IndexRefreshCacheHit:
			ccolor.Fprintf(s.stderrWriter(), " - Reused cached SDK index %s after fetch failed: %v\n", event.SDK, event.Err)
		}
	}
}

func splitTargets(args []string) []string {
	var targets []string
	for _, arg := range args {
		for _, part := range strings.Split(arg, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				targets = append(targets, part)
			}
		}
	}
	return targets
}

func (s *cliService) handleQuery(opts *QueryOptions) error {
	result, err := s.queryService.Query(app.QueryOptions{
		Repo:       opts.Target,
		Action:     opts.Action,
		Tag:        opts.Tag,
		Limit:      opts.Limit,
		JSON:       opts.JSON,
		Prerelease: opts.Prerelease,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		text, err := queryResultJSON(result)
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	}
	printQueryResult(result)
	return nil
}

func (s *cliService) handleSearch(opts *SearchOptions) error {
	result, err := s.searchService.Search(app.SearchOptions{
		Keyword: opts.Keyword,
		Extras:  opts.Extras,
		Limit:   opts.Limit,
		Sort:    opts.Sort,
		Order:   opts.Order,
	})
	if err != nil {
		return err
	}

	if opts.JSON {
		text, err := searchResultJSON(result)
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	}

	printSearchResult(result)
	return nil
}
