package install

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestBinaryChooserSkipsMetadataFiles(t *testing.T) {
	chooser := NewBinaryChooser("bd")

	t.Run("keeps extensionless binary candidate", func(t *testing.T) {
		direct, possible := chooser.Choose("bd", false, 0)
		assert.True(t, direct)
		assert.True(t, possible)
	})

	t.Run("skips extensionless metadata files", func(t *testing.T) {
		for _, name := range []string{"LICENSE", "README", "CHANGELOG", "NOTICE"} {
			direct, possible := chooser.Choose(name, false, 0)
			assert.False(t, direct)
			assert.False(t, possible)
		}
	})
}
