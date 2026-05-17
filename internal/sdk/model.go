package sdk

import "time"

type VersionKind string

const (
	VersionLatest VersionKind = "latest"
	VersionExact  VersionKind = "exact"
	VersionPrefix VersionKind = "prefix"
)

type Target struct {
	Raw     string
	Name    string
	Version string
	Kind    VersionKind
}

type Index struct {
	Schema    int         `json:"schema"`
	SDK       string      `json:"sdk"`
	SourceURL string      `json:"source_url"`
	FetchedAt time.Time   `json:"fetched_at"`
	Items     []IndexItem `json:"items"`
}

type IndexItem struct {
	Version string      `json:"version"`
	Stable  bool        `json:"stable"`
	Files   []IndexFile `json:"files"`
}

type IndexFile struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Ext      string `json:"ext"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
}
