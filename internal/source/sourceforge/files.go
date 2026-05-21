package sourceforge

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type FileType string

const (
	TypeFile      FileType = "f"
	TypeDirectory FileType = "d"
)

type File struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	DownloadURL  string   `json:"download_url"`
	URL          string   `json:"url"`
	FullPath     string   `json:"full_path"`
	Type         FileType `json:"type"`
	Downloadable bool     `json:"downloadable"`
	PublishedAt  time.Time
	ModTime      int64 `json:"mtime"`
	UpdatedAt    int64 `json:"updated"`
}

func ParseFilesPage(body []byte) ([]File, error) {
	const marker = "net.sf.files"

	start := bytes.Index(body, []byte(marker))
	if start < 0 {
		return nil, fmt.Errorf("sourceforge files data not found")
	}

	assign := bytes.IndexByte(body[start:], '=')
	if assign < 0 {
		return nil, fmt.Errorf("sourceforge files data not found")
	}

	objectStart := bytes.IndexByte(body[start+assign+1:], '{')
	if objectStart < 0 {
		return nil, fmt.Errorf("sourceforge files data not found")
	}
	objectStart += start + assign + 1

	objectEnd, err := findJSONObjectEnd(body, objectStart)
	if err != nil {
		return nil, err
	}

	filesByKey := make(map[string]File)
	if err := json.Unmarshal(body[objectStart:objectEnd], &filesByKey); err != nil {
		return nil, fmt.Errorf("parse sourceforge files data: %w", err)
	}
	if len(filesByKey) == 0 {
		return nil, fmt.Errorf("sourceforge files data not found")
	}

	modifiedTimes := parseModifiedTimes(body)
	keys := make([]string, 0, len(filesByKey))
	for key := range filesByKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := filesByKey[keys[i]], filesByKey[keys[j]]
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return keys[i] < keys[j]
	})

	files := make([]File, 0, len(keys))
	for _, key := range keys {
		file := filesByKey[key]
		file.PublishedAt = sourceForgeFileTime(file)
		if file.PublishedAt.IsZero() {
			if modifiedAt := modifiedTimes[file.Name]; !modifiedAt.IsZero() {
				file.PublishedAt = modifiedAt
			} else if modifiedAt := modifiedTimes[key]; !modifiedAt.IsZero() {
				file.PublishedAt = modifiedAt
			}
		}
		files = append(files, file)
	}
	return files, nil
}

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"`
	Media   rssMediaContent
}

type rssMediaContent struct {
	URL string `xml:"url,attr"`
}

func (m *rssMediaContent) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	if start.Name.Local != "content" {
		var skip struct{}
		return d.DecodeElement(&skip, &start)
	}
	for _, attr := range start.Attr {
		if attr.Name.Local == "url" {
			m.URL = attr.Value
			break
		}
	}
	var skip struct{}
	return d.DecodeElement(&skip, &start)
}

func ParseRSSFilesPage(body []byte) ([]File, error) {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse sourceforge rss data: %w", err)
	}
	if len(feed.Channel.Items) == 0 {
		return nil, fmt.Errorf("sourceforge rss data not found")
	}

	files := make([]File, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		link := strings.TrimSpace(item.Media.URL)
		if link == "" {
			link = strings.TrimSpace(item.Link)
		}
		fullPath := strings.Trim(strings.TrimSpace(item.Title), "/")
		if fullPath == "" {
			fullPath = fullPathFromDownloadURL(link)
		}
		if link == "" || fullPath == "" {
			continue
		}
		files = append(files, File{
			Name:         path.Base(fullPath),
			DownloadURL:  link,
			URL:          link,
			FullPath:     fullPath,
			Type:         TypeFile,
			Downloadable: true,
			PublishedAt:  parseRSSPubDate(item.PubDate),
		})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("sourceforge rss data not found")
	}
	return files, nil
}

func parseRSSPubDate(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC1123, "Mon, 02 Jan 2006 15:04:05 MST", "Mon, 02 Jan 2006 15:04:05 UT"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func fullPathFromDownloadURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] != "projects" || parts[i+2] != "files" {
			continue
		}
		fileParts := parts[i+3:]
		if len(fileParts) > 0 && fileParts[len(fileParts)-1] == "download" {
			fileParts = fileParts[:len(fileParts)-1]
		}
		for j, part := range fileParts {
			unescaped, err := url.PathUnescape(part)
			if err != nil {
				return ""
			}
			fileParts[j] = unescaped
		}
		return strings.Trim(strings.Join(fileParts, "/"), "/")
	}
	return ""
}

func sourceForgeFileTime(file File) time.Time {
	if file.ModTime > 0 {
		return time.Unix(file.ModTime, 0).UTC()
	}
	if file.UpdatedAt > 0 {
		return time.Unix(file.UpdatedAt, 0).UTC()
	}
	return time.Time{}
}

func parseModifiedTimes(body []byte) map[string]time.Time {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	times := make(map[string]time.Time)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" {
			name := strings.TrimSpace(attrValue(node, "title"))
			modifiedAt := modifiedTimeFromRow(node)
			if name != "" && !modifiedAt.IsZero() {
				times[name] = modifiedAt
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return times
}

func modifiedTimeFromRow(row *html.Node) time.Time {
	if row.Type == html.ElementNode && row.Data == "td" && strings.Contains(attrValue(row, "headers"), "files_date_h") {
		if modifiedAt := firstAbbrTitleTime(row); !modifiedAt.IsZero() {
			return modifiedAt
		}
	}
	for child := row.FirstChild; child != nil; child = child.NextSibling {
		if modifiedAt := modifiedTimeFromRow(child); !modifiedAt.IsZero() {
			return modifiedAt
		}
	}
	return time.Time{}
}

func firstAbbrTitleTime(node *html.Node) time.Time {
	if node.Type == html.ElementNode && node.Data == "abbr" {
		modifiedAt, err := time.Parse("2006-01-02 15:04:05 MST", strings.TrimSpace(attrValue(node, "title")))
		if err == nil {
			return modifiedAt.UTC()
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if modifiedAt := firstAbbrTitleTime(child); !modifiedAt.IsZero() {
			return modifiedAt
		}
	}
	return time.Time{}
}

func attrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func findJSONObjectEnd(body []byte, start int) (int, error) {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(body); i++ {
		ch := body[i]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
		}
	}

	return 0, fmt.Errorf("sourceforge files data object is incomplete")
}
