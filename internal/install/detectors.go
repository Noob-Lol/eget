package install

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

type detectorChain struct {
	detectors []Detector
	system    Detector
}

type assetDetector struct {
	Asset string
	Anti  bool
	Regex *regexp.Regexp
}

type allDetector struct{}

type systemOS struct {
	name     string
	regex    *regexp.Regexp
	anti     *regexp.Regexp
	priority *regexp.Regexp
}

type systemArch struct {
	name  string
	regex *regexp.Regexp
}

type systemDetector struct {
	Os   systemOS
	Arch systemArch
}

func (dc *detectorChain) Detect(assets []string) (string, []string, error) {
	for _, d := range dc.detectors {
		choice, candidates, err := d.Detect(assets)
		if len(candidates) == 0 && err != nil {
			return "", nil, err
		}
		if len(candidates) == 0 {
			return choice, nil, nil
		}
		assets = candidates
	}
	choice, candidates, err := dc.system.Detect(assets)
	if len(candidates) == 0 && err != nil {
		return "", nil, err
	}
	if len(candidates) == 0 {
		return choice, nil, nil
	}
	return "", candidates, fmt.Errorf("%d candidates found for asset chain", len(candidates))
}

func (a *allDetector) Detect(assets []string) (string, []string, error) {
	if len(assets) == 1 {
		return assets[0], nil, nil
	}
	return "", assets, fmt.Errorf("%d matches found", len(assets))
}

func (s *assetDetector) Detect(assets []string) (string, []string, error) {
	var candidates []string
	for _, a := range assets {
		base := path.Base(a)
		if !s.Anti && base == s.Asset {
			return a, nil, nil
		}
		if !s.Anti {
			if s.matches(base) {
				candidates = append(candidates, a)
			}
		}
		if s.Anti && !s.matches(base) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil, nil
	}
	if len(candidates) > 1 {
		return "", candidates, fmt.Errorf("%d candidates found for asset `%s`", len(candidates), s.Asset)
	}
	return "", nil, fmt.Errorf("asset `%s` not found", s.Asset)
}

func (s *assetDetector) matches(base string) bool {
	if s.Regex != nil {
		return s.Regex.MatchString(base)
	}
	return strings.Contains(strings.ToLower(base), strings.ToLower(s.Asset))
}

func compileAssetRegex(expr string) (*regexp.Regexp, error) {
	return regexp.Compile(expr)
}

func (osv *systemOS) Match(s string) (bool, bool) {
	if osv.anti != nil && osv.anti.MatchString(s) {
		return false, false
	}
	if osv.priority != nil {
		return osv.regex.MatchString(s), osv.priority.MatchString(s)
	}
	return osv.regex.MatchString(s), false
}

func (a *systemArch) Match(s string) bool {
	return a.regex.MatchString(s)
}

func newSystemDetector(goos, goarch string) (*systemDetector, error) {
	osv, ok := installGOOSMap[goos]
	if !ok {
		return nil, fmt.Errorf("unsupported target OS: %s", goos)
	}
	arch, ok := installGOARCHMap[goarch]
	if !ok {
		return nil, fmt.Errorf("unsupported target arch: %s", goarch)
	}
	return &systemDetector{Os: osv, Arch: arch}, nil
}

func (d *systemDetector) Detect(assets []string) (string, []string, error) {
	var priority []string
	var matches []string
	var candidates []string
	all := make([]string, 0, len(assets))
	for _, a := range assets {
		if strings.HasSuffix(a, ".sha256") || strings.HasSuffix(a, ".sha256sum") {
			continue
		}
		osMatch, extra := d.Os.Match(a)
		if extra {
			priority = append(priority, a)
		}
		archMatch := d.Arch.Match(a)
		if osMatch && archMatch {
			matches = append(matches, a)
		}
		if osMatch {
			candidates = append(candidates, a)
		}
		all = append(all, a)
	}
	if len(priority) == 1 {
		return priority[0], nil, nil
	}
	if len(priority) > 1 {
		return "", priority, fmt.Errorf("%d priority matches found", len(matches))
	}
	if len(matches) == 1 {
		return matches[0], nil, nil
	}
	if len(matches) > 1 {
		return "", matches, fmt.Errorf("%d matches found", len(matches))
	}
	if len(candidates) == 1 {
		return candidates[0], nil, nil
	}
	if len(candidates) > 1 {
		return "", candidates, fmt.Errorf("%d candidates found (unsure architecture)", len(candidates))
	}
	if len(all) == 1 {
		return all[0], nil, nil
	}
	return "", all, fmt.Errorf("no candidates found")
}

var (
	installOSDarwin    = systemOS{name: "darwin", regex: regexp.MustCompile(`(?i)(darwin|mac.?(os)?|osx)`)}
	installOSWindows   = systemOS{name: "windows", regex: regexp.MustCompile(`(?i)([^r]win|windows)`)}
	installOSLinux     = systemOS{name: "linux", regex: regexp.MustCompile(`(?i)(linux|ubuntu)`), anti: regexp.MustCompile(`(?i)(android)`), priority: regexp.MustCompile(`\.appimage$`)}
	installOSNetBSD    = systemOS{name: "netbsd", regex: regexp.MustCompile(`(?i)(netbsd)`)}
	installOSFreeBSD   = systemOS{name: "freebsd", regex: regexp.MustCompile(`(?i)(freebsd)`)}
	installOSOpenBSD   = systemOS{name: "openbsd", regex: regexp.MustCompile(`(?i)(openbsd)`)}
	installOSAndroid   = systemOS{name: "android", regex: regexp.MustCompile(`(?i)(android)`)}
	installOSIllumos   = systemOS{name: "illumos", regex: regexp.MustCompile(`(?i)(illumos)`)}
	installOSSolaris   = systemOS{name: "solaris", regex: regexp.MustCompile(`(?i)(solaris)`)}
	installOSPlan9     = systemOS{name: "plan9", regex: regexp.MustCompile(`(?i)(plan9)`)}
	installArchAMD64   = systemArch{name: "amd64", regex: regexp.MustCompile(`(?i)(x64|amd64|x86(-|_)?64)`)}
	installArchI386    = systemArch{name: "386", regex: regexp.MustCompile(`(?i)(x32|amd32|x86(-|_)?32|i?386)`)}
	installArchArm     = systemArch{name: "arm", regex: regexp.MustCompile(`(?i)(arm32|armv6|arm\b)`)}
	installArchArm64   = systemArch{name: "arm64", regex: regexp.MustCompile(`(?i)(arm64|armv8|aarch64)`)}
	installArchRiscv64 = systemArch{name: "riscv64", regex: regexp.MustCompile(`(?i)(riscv64)`)}
)

var installGOOSMap = map[string]systemOS{
	"darwin":  installOSDarwin,
	"windows": installOSWindows,
	"linux":   installOSLinux,
	"netbsd":  installOSNetBSD,
	"openbsd": installOSOpenBSD,
	"freebsd": installOSFreeBSD,
	"android": installOSAndroid,
	"illumos": installOSIllumos,
	"solaris": installOSSolaris,
	"plan9":   installOSPlan9,
}

var installGOARCHMap = map[string]systemArch{
	"amd64":   installArchAMD64,
	"386":     installArchI386,
	"arm":     installArchArm,
	"arm64":   installArchArm64,
	"riscv64": installArchRiscv64,
}
