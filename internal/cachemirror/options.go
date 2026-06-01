package cachemirror

import (
	"strings"
	"time"
)

const defaultTimeoutSeconds = 5

type Options struct {
	Enable   bool
	URL      string
	Timeout  time.Duration
	Fallback bool
}

func NormalizeOptions(opts Options) Options {
	opts.URL = strings.TrimRight(strings.TrimSpace(opts.URL), "/")
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeoutSeconds * time.Second
	}
	return opts
}

func (opts Options) Active() bool {
	opts = NormalizeOptions(opts)
	return opts.Enable && opts.URL != ""
}
