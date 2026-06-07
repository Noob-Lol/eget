package util

import (
	"os"
	"strings"
)

func GlobalProxyDisabled(noProxy bool) bool {
	if noProxy {
		return true
	}
	return strings.TrimSpace(os.Getenv("NO_PROXY")) != ""
}

func GlobalProxyDisabledWithEnv(noProxy bool, envNoProxy string) bool {
	if noProxy {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(envNoProxy)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
