package sourceforge

import (
	"bytes"
	"encoding/json"
	"fmt"
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
