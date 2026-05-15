package install

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestSystemDetectorSkipsSBOMAssets(t *testing.T) {
	d, err := newSystemDetector("windows", "amd64")
	assert.NoErr(t, err)

	got, candidates, err := d.Detect([]string{
		"https://example.com/vhs_0.11.0_Windows_x86_64.zip",
		"https://example.com/vhs_0.11.0_Windows_x86_64.zip.sbom.json",
	})

	assert.NoErr(t, err)
	assert.Eq(t, "https://example.com/vhs_0.11.0_Windows_x86_64.zip", got)
	assert.Empty(t, candidates)
}
