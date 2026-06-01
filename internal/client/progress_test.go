package client

import (
	"testing"
	"time"

	"github.com/gookit/goutil/testutil/assert"
)

func TestDownloadProgressFlushIntervalIsThrottled(t *testing.T) {
	assert.True(t, downloadProgressFlushInterval >= 500*time.Millisecond)
}
