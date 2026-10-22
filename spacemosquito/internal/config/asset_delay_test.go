package config

import (
	"testing"
	"time"
)

func TestStorageConfig_AssetDelay(t *testing.T) {
	for _, tc := range []struct {
		ms   int
		want time.Duration
	}{{0, 2500 * time.Millisecond}, {800, 800 * time.Millisecond}, {-1, 0}} {
		if got := (StorageConfig{AssetDelayMS: tc.ms}).AssetDelay(); got != tc.want {
			t.Errorf("asset_delay_ms %d: %v, want %v", tc.ms, got, tc.want)
		}
	}
}
