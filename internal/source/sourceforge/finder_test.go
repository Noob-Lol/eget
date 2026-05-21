package sourceforge

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

type fakeGetter struct {
	responses map[string]string
	requests  []string
}

type fakeHTTPGetterFunc func(url string) (*http.Response, error)

func (f fakeHTTPGetterFunc) Get(url string) (*http.Response, error) {
	return f(url)
}

func (g *fakeGetter) Get(url string) (*http.Response, error) {
	g.requests = append(g.requests, url)
	return htmlResponse(g.responses[url]), nil
}

func htmlResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func statusResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFinderFindsLatestFilesUnderSourcePath(t *testing.T) {
	firstURL := "https://sourceforge.net/projects/winmerge/files/stable/"
	secondURL := "https://sourceforge.net/projects/winmerge/files/stable/2.16.44/"
	getter := &fakeGetter{responses: map[string]string{
		firstURL: `
<script>
net.sf.files = {
  "2.16.42": {"name":"2.16.42","full_path":"/stable/2.16.42","type":"d"},
  "2.16.44": {"name":"2.16.44","full_path":"/stable/2.16.44","type":"d"}
};
</script>`,
		secondURL: `
<script>
net.sf.files = {
  "WinMerge-2.16.44-x64-Setup.exe": {
    "name":"WinMerge-2.16.44-x64-Setup.exe",
    "download_url":"https://downloads.sourceforge.net/project/winmerge/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe",
    "url":"https://sourceforge.net/projects/winmerge/files/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe/download",
    "full_path":"/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe",
    "type":"f"
  }
};
</script>`,
	}}

	urls, err := Finder{Project: "winmerge", Path: "stable", Getter: getter}.Find()

	if err != nil {
		t.Fatalf("Find(): %v", err)
	}
	assert.Len(t, urls, 1)
	assert.Eq(t, "https://downloads.sourceforge.net/project/winmerge/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe", urls[0])
	assert.Len(t, getter.requests, 2)
}

func TestFinderNormalizesSourceForgeDownloadPageURL(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://sourceforge.net/projects/winmerge/files/stable/2.16.56/": `
<script>
net.sf.files = {
  "WinMerge-2.16.56-x64-Setup.exe": {
    "name":"WinMerge-2.16.56-x64-Setup.exe",
    "download_url":"https://sourceforge.net/projects/winmerge/files/stable/2.16.56/WinMerge-2.16.56-x64-Setup.exe/download",
    "url":"/projects/winmerge/files/stable/2.16.56/WinMerge-2.16.56-x64-Setup.exe/",
    "full_path":"/stable/2.16.56/WinMerge-2.16.56-x64-Setup.exe",
    "type":"f"
  }
};
</script>`,
	}}

	urls, err := Finder{Project: "winmerge", Path: "stable/2.16.56", Getter: getter}.Find()

	assert.NoErr(t, err)
	assert.Eq(t, []string{"https://downloads.sourceforge.net/project/winmerge/stable/2.16.56/WinMerge-2.16.56-x64-Setup.exe"}, urls)
}

func TestFinderFallsBackToRSSWhenFilesPageIsForbidden(t *testing.T) {
	getter := fakeHTTPGetterFunc(func(rawURL string) (*http.Response, error) {
		switch rawURL {
		case "https://sourceforge.net/projects/victoria-ssd-hdd/files/":
			return statusResponse(http.StatusForbidden, "cf challenge"), nil
		case "https://sourceforge.net/projects/victoria-ssd-hdd/rss?path=/":
			return htmlResponse(`<?xml version="1.0" encoding="utf-8"?>
<rss xmlns:media="http://video.search.yahoo.com/mrss/" version="2.0">
  <channel>
    <item>
      <title><![CDATA[/Victoria537.zip]]></title>
      <link>https://sourceforge.net/projects/victoria-ssd-hdd/files/Victoria537.zip/download</link>
      <pubDate>Mon, 20 Oct 2025 01:21:41 UT</pubDate>
      <media:content url="https://sourceforge.net/projects/victoria-ssd-hdd/files/Victoria537.zip/download" />
    </item>
  </channel>
</rss>`), nil
		default:
			t.Fatalf("unexpected request %s", rawURL)
			return nil, nil
		}
	})

	urls, err := Finder{Project: "victoria-ssd-hdd", Getter: getter}.Find()

	assert.NoErr(t, err)
	assert.Eq(t, []string{"https://downloads.sourceforge.net/project/victoria-ssd-hdd/Victoria537.zip"}, urls)
}

func TestFinderEscapesSourcePathURLSegments(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.59/": `
<script>
net.sf.files = {
  "Ukrainian.zip": {
    "name":"Ukrainian.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.59/Ukrainian.zip",
    "full_path":"/Translations 2.x/2.59/Ukrainian.zip",
    "type":"f"
  }
};
</script>`,
	}}

	_, err := Finder{Project: "keepass", Path: "Translations 2.x", Tag: "2.59", Getter: getter}.Find()

	assert.NoErr(t, err)
	assert.Eq(t, []string{"https://sourceforge.net/projects/keepass/files/Translations%202.x/2.59/"}, getter.requests)
}

func TestFinderWithoutPathPrefersStableDirectory(t *testing.T) {
	rootURL := "https://sourceforge.net/projects/winmerge/files/"
	stableURL := "https://sourceforge.net/projects/winmerge/files/stable/"
	versionURL := "https://sourceforge.net/projects/winmerge/files/stable/2.16.44/"
	getter := &fakeGetter{responses: map[string]string{
		rootURL: `
<script>
net.sf.files = {
  "beta": {"name":"beta","full_path":"/beta","type":"d"},
  "stable": {"name":"stable","full_path":"/stable","type":"d"}
};
</script>`,
		stableURL: `
<script>
net.sf.files = {
  "2.16.42": {"name":"2.16.42","full_path":"/stable/2.16.42","type":"d"},
  "2.16.44": {"name":"2.16.44","full_path":"/stable/2.16.44","type":"d"}
};
</script>`,
		versionURL: `
<script>
net.sf.files = {
  "WinMerge-2.16.44-x64-Setup.exe": {
    "name":"WinMerge-2.16.44-x64-Setup.exe",
    "download_url":"https://downloads.sourceforge.net/project/winmerge/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe",
    "full_path":"/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe",
    "type":"f"
  }
};
</script>`,
	}}

	urls, err := Finder{Project: "winmerge", Getter: getter}.Find()

	assert.NoErr(t, err)
	assert.Eq(t, []string{"https://downloads.sourceforge.net/project/winmerge/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe"}, urls)
	assert.Eq(t, []string{rootURL, stableURL, versionURL}, getter.requests)
}

func TestLatestVersionUsesSourcePath(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://sourceforge.net/projects/winmerge/files/stable/": `
<script>
net.sf.files = {
  "2.16.42": {"name":"2.16.42","full_path":"/stable/2.16.42","type":"d"},
  "2.16.44": {"name":"2.16.44","full_path":"/stable/2.16.44","type":"d"}
};
</script>`,
		"https://sourceforge.net/projects/winmerge/files/stable/2.16.44/": `
<script>
net.sf.files = {
  "WinMerge-2.16.44-x64-Setup.exe": {
    "name":"WinMerge-2.16.44-x64-Setup.exe",
    "download_url":"https://downloads.sourceforge.net/project/winmerge/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe",
    "full_path":"/stable/2.16.44/WinMerge-2.16.44-x64-Setup.exe",
    "type":"f",
    "mtime":1770110419
  },
  "WinMerge-2.16.44-x64-Portable.zip": {
    "name":"WinMerge-2.16.44-x64-Portable.zip",
    "download_url":"https://downloads.sourceforge.net/project/winmerge/stable/2.16.44/WinMerge-2.16.44-x64-Portable.zip",
    "full_path":"/stable/2.16.44/WinMerge-2.16.44-x64-Portable.zip",
    "type":"f",
    "mtime":1770110500
  }
};
</script>`,
	}}

	info, err := LatestVersion("winmerge", "stable", getter)

	assert.NoErr(t, err)
	assert.Eq(t, []string{
		"https://sourceforge.net/projects/winmerge/files/stable/",
		"https://sourceforge.net/projects/winmerge/files/stable/2.16.44/",
	}, getter.requests)
	assert.Eq(t, "2.16.44", info.Version)
	assert.Eq(t, "/stable/2.16.44", info.Path)
	assert.Eq(t, 2, info.AssetsCount)
	assert.Eq(t, time.Date(2026, 2, 3, 9, 21, 40, 0, time.UTC), info.PublishedAt)
}

func TestLatestVersionFallsBackToLatestDownloadableFile(t *testing.T) {
	getter := fakeHTTPGetterFunc(func(rawURL string) (*http.Response, error) {
		switch rawURL {
		case "https://sourceforge.net/projects/victoria-ssd-hdd/files/":
			return statusResponse(http.StatusForbidden, "cf challenge"), nil
		case "https://sourceforge.net/projects/victoria-ssd-hdd/rss?path=/":
			return htmlResponse(`<?xml version="1.0" encoding="utf-8"?>
<rss xmlns:media="http://video.search.yahoo.com/mrss/" version="2.0">
  <channel>
    <item>
      <title><![CDATA[/Victoria536.zip]]></title>
      <link>https://sourceforge.net/projects/victoria-ssd-hdd/files/Victoria536.zip/download</link>
      <pubDate>Mon, 01 Sep 2025 01:21:41 UT</pubDate>
      <media:content url="https://sourceforge.net/projects/victoria-ssd-hdd/files/Victoria536.zip/download" />
    </item>
    <item>
      <title><![CDATA[/Victoria537.zip]]></title>
      <link>https://sourceforge.net/projects/victoria-ssd-hdd/files/Victoria537.zip/download</link>
      <pubDate>Mon, 20 Oct 2025 01:21:41 UT</pubDate>
      <media:content url="https://sourceforge.net/projects/victoria-ssd-hdd/files/Victoria537.zip/download" />
    </item>
  </channel>
</rss>`), nil
		default:
			t.Fatalf("unexpected request %s", rawURL)
			return nil, nil
		}
	})

	info, err := LatestVersion("victoria-ssd-hdd", "", getter)

	assert.NoErr(t, err)
	assert.Eq(t, "Victoria537.zip", info.Tag)
	assert.Eq(t, "Victoria537", info.Version)
	assert.Eq(t, "Victoria537.zip", info.Path)
	assert.Eq(t, 2, info.AssetsCount)
	assert.Eq(t, time.Date(2025, 10, 20, 1, 21, 41, 0, time.UTC), info.PublishedAt)
}

func TestLatestVersionUsesModifiedColumnTime(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/": `
<script>
net.sf.files = {
  "qbittorrent-5.1.2": {"name":"qbittorrent-5.1.2","full_path":"qbittorrent-win32/qbittorrent-5.1.2","type":"d"},
  "qbittorrent-5.2.0": {"name":"qbittorrent-5.2.0","full_path":"qbittorrent-win32/qbittorrent-5.2.0","type":"d"}
};
</script>`,
		"https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/qbittorrent-5.2.0/": `
<table>
  <tr title="qbittorrent_5.2.0_lt20_x64_setup.exe" class="file ">
    <th headers="files_name_h"><span class="name">qbittorrent_5.2.0_lt20_x64_setup.exe</span></th>
    <td headers="files_date_h" class="opt"><abbr title="2026-05-03 19:10:01 UTC">2026-05-03</abbr></td>
  </tr>
  <tr title="qbittorrent_5.2.0_x64_setup.exe" class="file ">
    <th headers="files_name_h"><span class="name">qbittorrent_5.2.0_x64_setup.exe</span></th>
    <td headers="files_date_h" class="opt"><abbr title="2026-05-03 19:10:36 UTC">2026-05-03</abbr></td>
  </tr>
</table>
<script>
net.sf.files = {
  "qbittorrent_5.2.0_lt20_x64_setup.exe": {
    "name":"qbittorrent_5.2.0_lt20_x64_setup.exe",
    "download_url":"https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/qbittorrent-5.2.0/qbittorrent_5.2.0_lt20_x64_setup.exe/download",
    "full_path":"qbittorrent-win32/qbittorrent-5.2.0/qbittorrent_5.2.0_lt20_x64_setup.exe",
    "type":"f"
  },
  "qbittorrent_5.2.0_x64_setup.exe": {
    "name":"qbittorrent_5.2.0_x64_setup.exe",
    "download_url":"https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/qbittorrent-5.2.0/qbittorrent_5.2.0_x64_setup.exe/download",
    "full_path":"qbittorrent-win32/qbittorrent-5.2.0/qbittorrent_5.2.0_x64_setup.exe",
    "type":"f"
  }
};
</script>`,
	}}

	info, err := LatestVersion("qbittorrent", "qbittorrent-win32", getter)

	assert.NoErr(t, err)
	assert.Eq(t, "5.2.0", info.Version)
	assert.Eq(t, "qbittorrent-win32/qbittorrent-5.2.0", info.Path)
	assert.Eq(t, 2, info.AssetsCount)
	assert.Eq(t, time.Date(2026, 5, 3, 19, 10, 36, 0, time.UTC), info.PublishedAt)
}

func TestListReleasesUsesSourcePathAndLimit(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/": `
<table>
  <tr title="qbittorrent-5.2.0" class="folder ">
    <td headers="files_date_h"><abbr title="2026-05-03 19:10:36 UTC">2026-05-03</abbr></td>
  </tr>
  <tr title="qbittorrent-5.2.0beta1" class="folder ">
    <td headers="files_date_h"><abbr title="2025-12-31 21:39:46 UTC">2025-12-31</abbr></td>
  </tr>
  <tr title="qbittorrent-5.1.2" class="folder ">
    <td headers="files_date_h"><abbr title="2025-11-01 10:00:00 UTC">2025-11-01</abbr></td>
  </tr>
</table>
<script>
net.sf.files = {
  "qbittorrent-5.1.2": {"name":"qbittorrent-5.1.2","full_path":"qbittorrent-win32/qbittorrent-5.1.2","type":"d"},
  "qbittorrent-5.2.0beta1": {"name":"qbittorrent-5.2.0beta1","full_path":"qbittorrent-win32/qbittorrent-5.2.0beta1","type":"d"},
  "qbittorrent-5.2.0": {"name":"qbittorrent-5.2.0","full_path":"qbittorrent-win32/qbittorrent-5.2.0","type":"d"}
};
</script>`,
	}}

	releases, err := ListReleases("qbittorrent", "qbittorrent-win32", 1, false, getter)

	if err != nil {
		t.Fatalf("ListReleases(): %v", err)
	}
	assert.Eq(t, []string{"https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/"}, getter.requests)
	assert.Len(t, releases, 1)
	assert.Eq(t, "qbittorrent-5.2.0", releases[0].Tag)
	assert.Eq(t, "5.2.0", releases[0].Version)
	assert.Eq(t, "qbittorrent-win32/qbittorrent-5.2.0", releases[0].Path)
	assert.Eq(t, 0, releases[0].AssetsCount)
	assert.Eq(t, time.Date(2026, 5, 3, 19, 10, 36, 0, time.UTC), releases[0].PublishedAt)
}

func TestListReleasesIncludesPrereleasesWhenRequested(t *testing.T) {
	getter := &fakeGetter{responses: map[string]string{
		"https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/": `
<script>
net.sf.files = {
  "qbittorrent-5.2.0": {"name":"qbittorrent-5.2.0","full_path":"qbittorrent-win32/qbittorrent-5.2.0","type":"d"},
  "qbittorrent-5.2.0beta1": {"name":"qbittorrent-5.2.0beta1","full_path":"qbittorrent-win32/qbittorrent-5.2.0beta1","type":"d"}
};
</script>`,
	}}

	releases, err := ListReleases("qbittorrent", "qbittorrent-win32", 2, true, getter)

	if err != nil {
		t.Fatalf("ListReleases(): %v", err)
	}
	assert.Len(t, releases, 2)
	assert.Eq(t, "qbittorrent-5.2.0", releases[0].Tag)
	assert.False(t, releases[0].Prerelease)
	assert.Eq(t, "qbittorrent-5.2.0beta1", releases[1].Tag)
	assert.True(t, releases[1].Prerelease)
	assert.Eq(t, []string{"https://sourceforge.net/projects/qbittorrent/files/qbittorrent-win32/"}, getter.requests)
}

func TestFinderFallbackVersionAssetsSkipsLatestAndScansOlderVersions(t *testing.T) {
	baseURL := "https://sourceforge.net/projects/keepass/files/Translations%202.x/"
	version260URL := "https://sourceforge.net/projects/keepass/files/Translations%202.x/2.60/"
	version259URL := "https://sourceforge.net/projects/keepass/files/Translations%202.x/2.59/"
	getter := &fakeGetter{responses: map[string]string{
		baseURL: `
<script>
net.sf.files = {
  "2.59": {"name":"2.59","full_path":"/Translations 2.x/2.59","type":"d"},
  "2.60": {"name":"2.60","full_path":"/Translations 2.x/2.60","type":"d"},
  "2.61": {"name":"2.61","full_path":"/Translations 2.x/2.61","type":"d"}
};
</script>`,
		version260URL: `
<script>
net.sf.files = {
  "Spanish.zip": {
    "name":"Spanish.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.60/Spanish.zip",
    "full_path":"/Translations 2.x/2.60/Spanish.zip",
    "type":"f"
  }
};
</script>`,
		version259URL: `
<script>
net.sf.files = {
  "Ukrainian.zip": {
    "name":"Ukrainian.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.59/Ukrainian.zip",
    "full_path":"/Translations 2.x/2.59/Ukrainian.zip",
    "type":"f"
  }
};
</script>`,
	}}

	assets, err := Finder{Project: "keepass", Path: "Translations 2.x", Getter: getter}.FallbackVersionAssets(2)

	assert.NoErr(t, err)
	assert.Eq(t, [][]string{
		{"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.60/Spanish.zip"},
		{"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.59/Ukrainian.zip"},
	}, assets)
	assert.Eq(t, []string{baseURL, version260URL, version259URL}, getter.requests)
}

func TestFinderFallbackVersionAssetsHonorsLimit(t *testing.T) {
	baseURL := "https://sourceforge.net/projects/keepass/files/Translations%202.x/"
	version260URL := "https://sourceforge.net/projects/keepass/files/Translations%202.x/2.60/"
	getter := &fakeGetter{responses: map[string]string{
		baseURL: `
<script>
net.sf.files = {
  "2.59": {"name":"2.59","full_path":"/Translations 2.x/2.59","type":"d"},
  "2.60": {"name":"2.60","full_path":"/Translations 2.x/2.60","type":"d"},
  "2.61": {"name":"2.61","full_path":"/Translations 2.x/2.61","type":"d"}
};
</script>`,
		version260URL: `
<script>
net.sf.files = {
  "Spanish.zip": {
    "name":"Spanish.zip",
    "download_url":"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.60/Spanish.zip",
    "full_path":"/Translations 2.x/2.60/Spanish.zip",
    "type":"f"
  }
};
</script>`,
	}}

	assets, err := Finder{Project: "keepass", Path: "Translations 2.x", Getter: getter}.FallbackVersionAssets(1)

	assert.NoErr(t, err)
	assert.Eq(t, [][]string{{"https://downloads.sourceforge.net/project/keepass/Translations%202.x/2.60/Spanish.zip"}}, assets)
	assert.Eq(t, []string{baseURL, version260URL}, getter.requests)
}
