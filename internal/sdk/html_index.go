package sdk

import (
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type HTMLParseOptions struct {
	SDK             string
	SourceURL       string
	IndexPathPrefix string
	FilenamePattern string
	Now             func() time.Time
}

func ParseHTMLIndex(body io.Reader, opts HTMLParseOptions) (Index, error) {
	base, err := url.Parse(opts.SourceURL)
	if err != nil {
		return Index{}, err
	}
	pattern := opts.FilenamePattern
	if pattern == "" {
		pattern = defaultFilenamePattern(opts.SDK)
	}
	matcher, err := compileFilenamePattern(pattern)
	if err != nil {
		return Index{}, err
	}

	items := map[string]*IndexItem{}
	tokenizer := html.NewTokenizer(body)
	for {
		typ := tokenizer.Next()
		if typ == html.ErrorToken {
			if tokenizer.Err() == io.EOF {
				break
			}
			return Index{}, tokenizer.Err()
		}
		if typ != html.StartTagToken && typ != html.SelfClosingTagToken {
			continue
		}
		token := tokenizer.Token()
		if token.Data != "a" {
			continue
		}
		href := attrValue(token, "href")
		if href == "" {
			continue
		}
		resolved, ok := resolveIndexHref(base, href, opts.IndexPathPrefix)
		if !ok {
			continue
		}
		parsed, ok := matcher.Match(path.Base(resolved.Path))
		if !ok {
			continue
		}
		file := parsed.File
		file.URL = resolved.String()
		item := items[parsed.Version]
		if item == nil {
			item = &IndexItem{Version: parsed.Version, Stable: isStableVersion(parsed.Version)}
			items[parsed.Version] = item
		}
		item.Files = append(item.Files, file)
	}
	if len(items) == 0 {
		return Index{}, fmt.Errorf("no sdk files found in html index")
	}

	indexItems := make([]IndexItem, 0, len(items))
	for _, item := range items {
		sort.Slice(item.Files, func(i, j int) bool {
			return item.Files[i].Filename < item.Files[j].Filename
		})
		indexItems = append(indexItems, *item)
	}
	sort.Slice(indexItems, func(i, j int) bool {
		return compareVersion(indexItems[i].Version, indexItems[j].Version) < 0
	})

	return newIndex(opts.SDK, opts.SourceURL, opts.Now, indexItems), nil
}

func attrValue(token html.Token, name string) string {
	for _, attr := range token.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func resolveIndexHref(base *url.URL, href, prefix string) (*url.URL, bool) {
	ref, err := url.Parse(href)
	if err != nil {
		return nil, false
	}
	resolved := base.ResolveReference(ref)
	if prefix != "" && !strings.HasPrefix(resolved.Path, prefix) {
		return nil, false
	}
	return resolved, true
}

func defaultFilenamePattern(name string) string {
	switch name {
	case "go":
		return "go{version}.{os}-{arch}.{ext}"
	case "node":
		return "node-v{version}-{os}-{arch}.{ext}"
	default:
		return "{version}-{os}-{arch}.{ext}"
	}
}

type filenameMatcher struct {
	re *regexp.Regexp
}

type parsedIndexFile struct {
	Version string
	File    IndexFile
}

func compileFilenamePattern(pattern string) (filenameMatcher, error) {
	var out strings.Builder
	out.WriteByte('^')
	for i := 0; i < len(pattern); {
		if pattern[i] != '{' {
			out.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			i++
			continue
		}
		end := strings.IndexByte(pattern[i:], '}')
		if end < 0 {
			return filenameMatcher{}, fmt.Errorf("invalid filename pattern %q", pattern)
		}
		name := pattern[i+1 : i+end]
		switch name {
		case "version":
			out.WriteString(`(?P<version>[^/\s]+?)`)
		case "os":
			out.WriteString(`(?P<os>[^-/.\s]+)`)
		case "arch":
			out.WriteString(`(?P<arch>[^/.\s]+)`)
		case "ext":
			out.WriteString(`(?P<ext>.+)`)
		default:
			return filenameMatcher{}, fmt.Errorf("unsupported filename pattern variable %q", name)
		}
		i += end + 1
	}
	out.WriteByte('$')
	re, err := regexp.Compile(out.String())
	return filenameMatcher{re: re}, err
}

func (m filenameMatcher) Match(filename string) (parsedIndexFile, bool) {
	matches := m.re.FindStringSubmatch(filename)
	if matches == nil {
		return parsedIndexFile{}, false
	}
	values := map[string]string{}
	for i, name := range m.re.SubexpNames() {
		if i > 0 && name != "" {
			values[name] = matches[i]
		}
	}
	if values["version"] == "" || values["os"] == "" || values["arch"] == "" || values["ext"] == "" {
		return parsedIndexFile{}, false
	}
	return parsedIndexFile{
		Version: values["version"],
		File: IndexFile{
			OS:       values["os"],
			Arch:     values["arch"],
			Ext:      values["ext"],
			Filename: filename,
		},
	}, true
}

func isStableVersion(version string) bool {
	return !strings.Contains(version, "-")
}

func compareVersion(a, b string) int {
	if a == b {
		return 0
	}
	ap := strings.Split(strings.SplitN(a, "-", 2)[0], ".")
	bp := strings.Split(strings.SplitN(b, "-", 2)[0], ".")
	limit := len(ap)
	if len(bp) > limit {
		limit = len(bp)
	}
	for i := 0; i < limit; i++ {
		av := versionPart(ap, i)
		bv := versionPart(bp, i)
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	if a < b {
		return -1
	}
	return 1
}

func versionPart(parts []string, idx int) int {
	if idx >= len(parts) {
		return 0
	}
	n := 0
	for _, ch := range parts[idx] {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
