package sourceforge

import (
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

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
