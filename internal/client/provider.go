package client

import (
	"net/url"
	"strings"
)

func isGitHubAPIRequest(u *url.URL) bool {
	return u != nil && strings.EqualFold(u.Host, "api.github.com")
}

func isProviderMetadataRequest(u *url.URL) bool {
	if u == nil {
		return false
	}
	switch {
	case isGitHubAPIRequest(u):
		return true
	case isGitLabAPIRequest(u):
		return true
	case isGiteaAPIRequest(u):
		return true
	case isSourceForgeFilesRequest(u):
		return true
	default:
		return false
	}
}

func isSourceForgeDownloadRequest(u *url.URL) bool {
	if u == nil {
		return false
	}
	if strings.EqualFold(u.Host, "downloads.sourceforge.net") {
		return true
	}
	if !strings.EqualFold(u.Host, "sourceforge.net") {
		return false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	return len(parts) >= 4 && parts[0] == "projects" && parts[2] == "files" && strings.EqualFold(parts[len(parts)-1], "download")
}

func isGitLabAPIRequest(u *url.URL) bool {
	if u == nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	return len(parts) >= 5 && parts[0] == "api" && parts[1] == "v4" && parts[2] == "projects" && parts[4] == "releases"
}

func isGiteaAPIRequest(u *url.URL) bool {
	if u == nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	return len(parts) >= 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "repos" && parts[5] == "releases"
}

func isSourceForgeFilesRequest(u *url.URL) bool {
	if u == nil || !strings.EqualFold(u.Host, "sourceforge.net") {
		return false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) < 3 || parts[0] != "projects" || parts[2] != "files" {
		return false
	}
	return !strings.EqualFold(parts[len(parts)-1], "download")
}

func isGitHubDownloadRequest(u *url.URL) bool {
	return u != nil && strings.EqualFold(u.Host, "github.com")
}
