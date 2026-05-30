package urltemplate

import (
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"path"
	"strings"
	"time"
)

type HTTPGetter interface {
	Get(url string) (*http.Response, error)
}

type Finder struct {
	Name    string
	Target  Target
	Config  Config
	Tag     string
	GOOS    string
	GOARCH  string
	Libc    string
	Getter  HTTPGetter
	Version string
	vars    map[string]string
}

type LatestInfo struct {
	Version     string
	PublishedAt time.Time
}

func (f *Finder) Find() ([]string, error) {
	if f.Config.URLTemplate == "" {
		return nil, fmt.Errorf("url_template is required")
	}
	version := f.Tag
	if version == "" {
		latest, err := f.Latest()
		if err != nil {
			return nil, err
		}
		version = latest.Version
	}
	vars, err := VariablesFor(VariableInput{
		Name:    f.Name,
		Version: version,
		GOOS:    f.GOOS,
		GOARCH:  f.GOARCH,
		Libc:    f.Libc,
		Config:  f.Config,
	})
	if err != nil {
		return nil, err
	}
	url, err := Render(f.Config.URLTemplate, vars)
	if err != nil {
		return nil, err
	}
	f.Version = version
	f.vars = vars
	return []string{url}, nil
}

func (f *Finder) Latest() (LatestInfo, error) {
	if f.Config.LatestURL == "" {
		return LatestInfo{}, fmt.Errorf("latest_url is required")
	}
	if f.Getter == nil {
		return LatestInfo{}, fmt.Errorf("http getter is required")
	}
	// Template latest/checksum URLs are arbitrary site metadata. They use the
	// shared HTTP client for proxy/SSL behavior, but do not force API-cache
	// classification because arbitrary metadata URLs are not provider APIs.
	resp, err := f.Getter.Get(f.Config.LatestURL)
	if err != nil {
		return LatestInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return LatestInfo{}, fmt.Errorf("fetch latest metadata: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return LatestInfo{}, err
	}
	cfg := f.Config
	if cfg.LatestFormat == "" {
		cfg.LatestFormat = inferMetadataFormat(f.Config.LatestURL)
	}
	version, err := ParseLatest(data, cfg)
	if err != nil {
		return LatestInfo{}, err
	}
	publishedAt, err := ParseLatestPublishedAt(data, cfg)
	if err != nil {
		return LatestInfo{}, err
	}
	return LatestInfo{Version: version, PublishedAt: publishedAt}, nil
}

func (f *Finder) Vars() map[string]string {
	cloned := make(map[string]string, len(f.vars))
	for key, value := range f.vars {
		cloned[key] = value
	}
	return cloned
}

func inferMetadataFormat(rawURL string) string {
	parsed, err := neturl.Parse(rawURL)
	targetPath := rawURL
	if err == nil && parsed.Path != "" {
		targetPath = parsed.Path
	}
	switch strings.ToLower(path.Ext(targetPath)) {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".txt":
		return "text"
	default:
		return ""
	}
}
