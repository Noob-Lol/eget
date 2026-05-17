package sdk

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
