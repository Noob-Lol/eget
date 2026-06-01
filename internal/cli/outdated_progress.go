package cli

import (
	"io"
	"os"
	"sync"

	"github.com/gookit/cliui/progress"
	"github.com/gookit/goutil/x/ccolor"
	"github.com/inherelab/eget/internal/client"
	"github.com/inherelab/eget/internal/install"
)

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
