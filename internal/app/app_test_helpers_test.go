package app

import (
	"testing"
	"time"
)

func waitForMaxActive(t *testing.T, current func() int, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for max active %d, got %d", want, current())
		case <-ticker.C:
			if current() >= want {
				return
			}
		}
	}
}
