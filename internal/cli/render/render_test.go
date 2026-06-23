package render

import (
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/inherelab/eget/internal/app"
)

func TestShowResultToDisplayUsesUpdatedAt(t *testing.T) {
	updatedAt := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)

	display := ShowResultToDisplay(app.ShowResult{
		Name:      "fzf",
		UpdatedAt: updatedAt,
	})

	assert.Eq(t, "2026-05-18 10:00:00", display.UpdatedAt)
}
