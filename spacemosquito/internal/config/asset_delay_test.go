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

func TestServeLogFile(t *testing.T) {
	empty, rel, abs := "", "my.log", "/var/log/sm.log"
	for _, tc := range []struct {
		v    *string
		want string
	}{{nil, "/data/logs/serve.log"}, {&empty, ""}, {&rel, "/data/my.log"}, {&abs, "/var/log/sm.log"}} {
		if got := (&Config{LogFile: tc.v}).ServeLogFile("/data"); got != tc.want {
			t.Errorf("log_file %v: %q, want %q", tc.v, got, tc.want)
		}
	}
}
