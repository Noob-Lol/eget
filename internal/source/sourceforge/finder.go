package sourceforge

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type HTTPGetter interface {
	Get(url string) (*http.Response, error)
}

type Finder struct {
	Project string
	Path    string
	Tag     string
	Getter  HTTPGetter
}

type LatestInfo struct {
	Tag         string
	Version     string
	Path        string
	PublishedAt time.Time
	Prerelease  bool
	AssetsCount int
}

func (f Finder) Find() ([]string, error) {
	if strings.TrimSpace(f.Project) == "" {
		return nil, fmt.Errorf("sourceforge project is required")
	}
	if f.Getter == nil {
		return nil, fmt.Errorf("sourceforge HTTP getter is required")
	}

	sourcePath := strings.Trim(strings.Trim(f.Path, "/")+"/"+strings.Trim(f.Tag, "/"), "/")
	files, err := f.list(sourcePath)
	if err != nil {
		return nil, err
	}

	urls := downloadableURLs(files)
	if len(urls) > 0 {
		return urls, nil
	}

	latest, ok := LatestVersionFile(files)
	if !ok {
		if sourcePath == "" {
			stable, stableOK := stableDirectory(files)
			if stableOK {
				files, err = f.list(stable.FullPath)
				if err != nil {
					return nil, err
				}
				latest, ok = LatestVersionFile(files)
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("sourceforge downloadable files not found")
	}
	files, err = f.list(latest.FullPath)
	if err != nil {
		return nil, err
	}

	urls = downloadableURLs(files)
	if len(urls) == 0 {
		return nil, fmt.Errorf("sourceforge downloadable files not found")
	}
	return urls, nil
}

func (f Finder) FallbackVersionAssets(limit int) ([][]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	if strings.TrimSpace(f.Project) == "" {
		return nil, fmt.Errorf("sourceforge project is required")
	}
	if f.Getter == nil {
		return nil, fmt.Errorf("sourceforge HTTP getter is required")
	}

	files, err := f.list(strings.Trim(f.Path, "/"))
	if err != nil {
		return nil, err
	}
	versions := sortedVersionDirectories(files)
	if len(versions) <= 1 {
		return nil, nil
	}

	var groups [][]string
	attempts := 0
	for _, version := range versions[1:] {
		if attempts >= limit {
			break
		}
		attempts++
		files, err := f.list(version.FullPath)
		if err != nil {
			return nil, err
		}
		urls := downloadableURLs(files)
		if len(urls) > 0 {
			groups = append(groups, urls)
		}
	}
	return groups, nil
}

func LatestVersion(project, sourcePath string, getter HTTPGetter) (LatestInfo, error) {
	finder := Finder{Project: project, Path: sourcePath, Getter: getter}
	files, err := finder.list(strings.Trim(sourcePath, "/"))
	if err != nil {
		return LatestInfo{}, err
	}

	versions, err := releaseVersionFiles(finder, files, sourcePath)
	if err != nil {
		return LatestInfo{}, err
	}
	if len(versions) == 0 {
		return LatestInfo{}, fmt.Errorf("could not determine SourceForge latest version for %s", project)
	}
	return releaseInfo(finder, versions[0])
}

func ListReleases(project, sourcePath string, limit int, includePrerelease bool, getter HTTPGetter) ([]LatestInfo, error) {
	if limit <= 0 {
		limit = 10
	}
	finder := Finder{Project: project, Path: sourcePath, Getter: getter}
	files, err := finder.list(strings.Trim(sourcePath, "/"))
	if err != nil {
		return nil, err
	}

	versions, err := releaseVersionFiles(finder, files, sourcePath)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("could not determine SourceForge releases for %s", project)
	}

	releases := make([]LatestInfo, 0, min(limit, len(versions)))
	for _, version := range versions {
		if len(releases) == limit {
			break
		}
		if !includePrerelease && isPrereleaseVersion(version) {
			continue
		}
		info, err := releaseInfo(finder, version)
		if err != nil {
			return nil, err
		}
		releases = append(releases, info)
	}
	return releases, nil
}

func releaseVersionFiles(finder Finder, files []File, sourcePath string) ([]File, error) {
	versions := sortedVersionDirectories(files)
	if len(versions) == 0 && sourcePath == "" {
		stable, stableOK := stableDirectory(files)
		if stableOK {
			var err error
			files, err = finder.list(stable.FullPath)
			if err != nil {
				return nil, err
			}
			versions = sortedVersionDirectories(files)
		}
	}
	return versions, nil
}

func releaseInfo(finder Finder, versionFile File) (LatestInfo, error) {
	version := fileVersion(versionFile)
	if version == "" {
		return LatestInfo{}, fmt.Errorf("could not determine SourceForge version from %s", versionFile.Name)
	}
	assets, err := finder.list(versionFile.FullPath)
	if err != nil {
		return LatestInfo{}, err
	}
	return LatestInfo{
		Tag:         sourceForgeReleaseTag(versionFile),
		Version:     version,
		Path:        versionFile.FullPath,
		PublishedAt: latestPublishedAt(assets),
		Prerelease:  isPrereleaseVersion(versionFile),
		AssetsCount: len(downloadableURLs(assets)),
	}, nil
}

func sourceForgeReleaseTag(file File) string {
	if file.Name != "" {
		return file.Name
	}
	return path.Base(strings.Trim(file.FullPath, "/"))
}

func isPrereleaseVersion(file File) bool {
	name := strings.ToLower(sourceForgeReleaseTag(file))
	return strings.Contains(name, "alpha") ||
		strings.Contains(name, "beta") ||
		strings.Contains(name, "rc") ||
		strings.Contains(name, "pre") ||
		strings.Contains(name, "preview")
}

func latestPublishedAt(files []File) time.Time {
	var latest time.Time
	for _, file := range files {
		if file.Type != TypeFile || file.DownloadURL == "" || file.PublishedAt.IsZero() {
			continue
		}
		if latest.IsZero() || file.PublishedAt.After(latest) {
			latest = file.PublishedAt
		}
	}
	return latest
}

func stableDirectory(files []File) (File, bool) {
	for _, file := range files {
		if file.Type == TypeDirectory && strings.EqualFold(strings.Trim(file.Name, "/"), "stable") {
			return file, true
		}
	}
	return File{}, false
}

func (f Finder) list(sourcePath string) ([]File, error) {
	url := "https://sourceforge.net/projects/" + strings.Trim(f.Project, "/") + "/files/"
	if sourcePath != "" {
		url += escapeSourcePath(sourcePath) + "/"
	}

	verbosef("sourceforge finder request: %s", url)
	resp, err := f.Getter.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sourceforge files page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	verbosef("sourceforge finder response: %s", truncateBody(body))
	return ParseFilesPage(body)
}

func downloadableURLs(files []File) []string {
	urls := make([]string, 0, len(files))
	for _, file := range files {
		if file.Type == TypeFile && file.DownloadURL != "" {
			urls = append(urls, directDownloadURL(file))
		}
	}
	return urls
}

func directDownloadURL(file File) string {
	if strings.TrimSpace(file.FullPath) == "" {
		return file.DownloadURL
	}
	parsed, err := url.Parse(file.DownloadURL)
	if err != nil {
		return file.DownloadURL
	}

	if parsed.Host == "sourceforge.net" && strings.HasSuffix(strings.Trim(parsed.Path, "/"), "/download") {
		project := projectFromDownloadPath(parsed.Path)
		if project != "" {
			return sourceForgeDownloadURL(project, file.FullPath)
		}
	}

	if parsed.Host == "downloads.sourceforge.net" {
		project := projectFromDownloadsPath(parsed.Path)
		if project != "" {
			return sourceForgeDownloadURL(project, file.FullPath)
		}
	}

	return file.DownloadURL
}

func sourceForgeDownloadURL(project, fullPath string) string {
	return "https://downloads.sourceforge.net/project/" + url.PathEscape(project) + "/" + escapeSourcePath(fullPath)
}

func escapeSourcePath(sourcePath string) string {
	parts := strings.Split(strings.Trim(sourcePath, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func projectFromDownloadPath(rawPath string) string {
	parts := strings.Split(strings.Trim(rawPath, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "projects" {
			return path.Clean(parts[i+1])
		}
	}
	return ""
}

func projectFromDownloadsPath(rawPath string) string {
	parts := strings.Split(strings.Trim(rawPath, "/"), "/")
	if len(parts) >= 2 && parts[0] == "project" {
		return path.Clean(parts[1])
	}
	return ""
}
