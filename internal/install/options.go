package install

import (
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/inherelab/eget/internal/source/forge"
	"github.com/inherelab/eget/internal/source/sourceforge"
	"github.com/inherelab/eget/internal/source/urltemplate"
)

type Options struct {
	Tag                 string
	Prerelease          bool
	Name                string
	Verbose             bool
	Source              bool
	SourcePath          string
	Sys7zPath           string
	Output              string
	OutputExplicit      bool
	GuiTarget           string
	IsGUI               bool
	InstallMode         string
	CacheDir            string
	CacheName           string
	CacheVersion        string
	ProxyURL            string
	APICacheEnabled     bool
	APICacheDir         string
	APICacheTime        int
	GhproxyEnabled      bool
	GhproxyHostURL      string
	GhproxySupportAPI   bool
	GhproxyFallbacks    []string
	System              string
	ExtractFile         string
	All                 bool
	Quiet               bool
	DownloadOnly        bool
	FallbackVersions    int
	ChunkConcurrency    int
	BatchConcurrency    int
	ChunkConcurrencySet bool
	BatchConcurrencySet bool
	UpgradeOnly         bool
	Asset               []string
	RenameFiles         map[string]string
	Hash                bool
	Verify              string
	URLTemplate         URLTemplateOptions
	DisableSSL          bool
}

type URLTemplateOptions struct {
	URLTemplate         string
	LatestURL           string
	LatestFormat        string
	LatestJSONPath      string
	VersionRegex        string
	OSMap               map[string]string
	ArchMap             map[string]string
	ExtMap              map[string]string
	LibcMap             map[string]string
	ChecksumURLTemplate string
	ChecksumFormat      string
	ChecksumJSONPath    string
	ChecksumRegex       string
	InstallAction       string
	InstallArgs         []string
	ResolvedVersion     string
	ResolvedVars        map[string]string
}

const (
	InstallModePortable  = "portable"
	InstallModeInstaller = "installer"
)

type TargetKind string

const (
	TargetUnknown     TargetKind = "unknown"
	TargetRepo        TargetKind = "repo"
	TargetGitHubURL   TargetKind = "github_url"
	TargetDirectURL   TargetKind = "direct_url"
	TargetLocalFile   TargetKind = "local_file"
	TargetSourceForge TargetKind = "sourceforge"
	TargetForge       TargetKind = "forge"
	TargetTemplate    TargetKind = "template"
)

var githubURLPattern = regexp.MustCompile(`^(http(s)?://)?github\.com/[\w\-_.,]+/[\w\-_.,]+(.git)?(/)?$`)

func IsURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func IsGitHubURL(s string) bool {
	return githubURLPattern.MatchString(s)
}

func IsLocalFile(s string) bool {
	_, err := os.Stat(s)
	return err == nil
}

func DetectTargetKind(target string) TargetKind {
	switch {
	case IsLocalFile(target):
		return TargetLocalFile
	case sourceforge.IsTarget(target):
		return TargetSourceForge
	case forge.IsTarget(target):
		return TargetForge
	case urltemplate.IsTarget(target):
		return TargetTemplate
	case IsGitHubURL(target):
		return TargetGitHubURL
	case IsURL(target):
		return TargetDirectURL
	case isRepoTarget(target):
		return TargetRepo
	default:
		return TargetUnknown
	}
}

func isRepoTarget(target string) bool {
	parts := strings.Split(target, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

func extractAllFromFileSpec(file string) bool {
	for _, part := range strings.Split(file, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(file, ",") {
			return true
		}
		if strings.ContainsAny(part, "*?[{") {
			return true
		}
	}
	return false
}
