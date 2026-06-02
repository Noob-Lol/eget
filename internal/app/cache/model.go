package cache

import "time"

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
	Path   string
	Reason string
}

type CleanResult struct {
	CacheDir     string
	MatchedFiles int
	RemovedFiles int
	MatchedSize  int64
	RemovedSize  int64
	Skipped      []CleanSkip
}

func (r CleanResult) NeedsConfirmation() bool {
	return r.MatchedFiles >= 100 || r.MatchedSize >= 1024*1024*1024
}

type ServeOptions struct {
	Host    string
	Port    int
	Root    string
	NoIndex bool
	Version string
	Token   string
}
