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
