package urltemplate

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func EffectiveSystem(system, currentGOOS, currentGOARCH string, detectLibc func() string, fixDarwin func(string, string) (string, string)) (string, string, string) {
	goos, goarch := currentGOOS, currentGOARCH
	explicit := false
	if system != "" {
		parts := strings.SplitN(system, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			goos, goarch = parts[0], parts[1]
			explicit = true
		}
	}
	if !explicit && fixDarwin != nil {
		goos, goarch = fixDarwin(goos, goarch)
	}

	libc := ""
	if goos == "linux" && detectLibc != nil {
		libc = detectLibc()
	}
	return goos, goarch, libc
}

func DetectLibc() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	for _, path := range []string{"/usr/bin/ldd", "/bin/ldd"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.ToLower(string(data))
		if strings.Contains(text, "musl") {
			return "musl"
		}
		if strings.Contains(text, "glibc") || strings.Contains(text, "gnu libc") {
			return "glibc"
		}
	}
	return "glibc"
}

func FixDarwinRosetta(goos, goarch string) (string, string) {
	if goos != "darwin" || goarch != "amd64" {
		return goos, goarch
	}
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return goos, goarch
	}
	if strings.TrimSpace(string(out)) == "arm64" {
		return "darwin", "arm64"
	}
	return goos, goarch
}
