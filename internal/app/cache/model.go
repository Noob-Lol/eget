package cache

import (
	"io"
	"time"
)

type Kind string

const (
	KindPkg      Kind = "pkg"
	KindAPI      Kind = "api"
	KindSDK      Kind = "sdk"
	KindSDKIndex Kind = "sdk-index"
	KindPartial  Kind = "partial"
)

var defaultCacheCleanKinds = []Kind{
	KindPkg,
	KindAPI,
	KindSDK,
	KindPartial,
}

type Entry struct {
	Kind      Kind
	Path      string
	RelPath   string
	Size      int64
	ModTime   time.Time
	IsPartial bool
}

type CleanOptions struct {
	Older  time.Duration
	All    bool
	DryRun bool
	Yes    bool
	Kinds  []Kind
}

type CleanSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type CleanResult struct {
	CacheDir     string      `json:"cache_dir"`
	MatchedFiles int         `json:"matched_files"`
	RemovedFiles int         `json:"removed_files"`
	MatchedSize  int64       `json:"matched_size"`
	RemovedSize  int64       `json:"removed_size"`
	Skipped      []CleanSkip `json:"skipped"`
}

func (r CleanResult) NeedsConfirmation() bool {
	return r.MatchedFiles >= 100 || r.MatchedSize >= 1024*1024*1024
}

type ServeOptions struct {
	Host      string
	Port      int
	Root      string
	NoIndex   bool
	Version   string
	Token     string
	JSONLog   bool
	LogWriter io.Writer `json:"-"`
}
