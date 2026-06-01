package sourceforge

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

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
