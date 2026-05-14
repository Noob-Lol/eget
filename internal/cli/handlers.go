package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gookit/cliui/progress"
	"github.com/gookit/cliui/show"
	"github.com/gookit/goutil/cliutil"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/app"
	"github.com/inherelab/eget/internal/client"
	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
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
	if opts != nil && opts.Info != "" {
		item, err := s.listService.FindPackage(opts.Info)
		if err != nil {
			return err
		}
		show.AList("Package Info", item)
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
			rows = append(rows, []any{item.Name, item.Repo, item.InstalledTag, item.LatestTag, formatListTime(item.PublishedAt)})
		}
		ccolor.Print(cliutil.FormatTable(cols, rows, cliutil.MinimalStyle))
		return nil
	}

	var items []app.ListItem
	var err error
	if opts != nil && opts.GUI {
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
	if value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339)
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
		ccolor.Printf("# %s, exists: %v\n", info.Path, info.Exists)
		show.MList(map[string]any{
			"global":   cfg.Global,
			"apiCache": cfg.ApiCache,
			"ghproxy":  cfg.Ghproxy,
		})
		ccolor.Yellowln("📦 Configed Packages:")
		show.MList(cfg.Packages)
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
		text, err := result.JSONString()
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
		text, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(text))
		return nil
	}

	printSearchResult(result)
	return nil
}
