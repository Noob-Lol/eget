package sourceforge

import (
	"net/http"
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

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
