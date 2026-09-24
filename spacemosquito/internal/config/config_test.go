package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_missingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestLoad_malformedYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("database: [invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoad_defaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `
database: {}
storage: {}
session:
  encryption_key: "12345678901234567890123456789012"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Database.Driver != "sqlite" {
		t.Errorf("database.driver = %q, want sqlite", cfg.Database.Driver)
	}
	if cfg.Database.DriverName() != "sqlite" {
		t.Errorf("DriverName() = %q, want sqlite", cfg.Database.DriverName())
	}
	if cfg.Storage.BasePath != "./saved" {
		t.Errorf("storage.base_path = %q, want ./saved", cfg.Storage.BasePath)
	}
	if cfg.MCP.Port != 56403 {
		t.Errorf("mcp.port = %d, want 56403", cfg.MCP.Port)
	}
	if cfg.MCP.Timeout != 3600 {
		t.Errorf("mcp.session_timeout = %d, want 3600", cfg.MCP.Timeout)
	}
	if cfg.Embedder.Model != "nomic-embed-text" {
		t.Errorf("embedder.model = %q, want nomic-embed-text", cfg.Embedder.Model)
	}
}

func TestLoad_cronDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `
database: {}
session:
  encryption_key: "12345678901234567890123456789012"
cron:
  full_crawl:
    enabled: true
  incremental:
    enabled: true
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Cron.FullCrawl.Interval != "24h" {
		t.Errorf("full_crawl.interval = %q, want 24h", cfg.Cron.FullCrawl.Interval)
	}
	if cfg.Cron.Incremental.Interval != "2h" {
		t.Errorf("incremental.interval = %q, want 2h", cfg.Cron.Incremental.Interval)
	}
	if cfg.Cron.Incremental.Detection != "dom" {
		t.Errorf("incremental.detection = %q, want dom", cfg.Cron.Incremental.Detection)
	}
	if cfg.Cron.FullCrawl.MaxDuration != "4h" {
		t.Errorf("full_crawl.max_duration = %q, want 4h", cfg.Cron.FullCrawl.MaxDuration)
	}
	if cfg.Cron.Incremental.MaxDuration != "30m" {
		t.Errorf("incremental.max_duration = %q, want 30m", cfg.Cron.Incremental.MaxDuration)
	}
}

func TestParseCronDuration(t *testing.T) {
	d, err := ParseCronDuration("6h")
	if err != nil {
		t.Fatal(err)
	}
	if d != 6*time.Hour {
		t.Errorf("got %v, want 6h", d)
	}

	_, err = ParseCronDuration("not-a-duration")
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestCronJobConfig_ParseMaxDuration(t *testing.T) {
	var nilCfg *CronJobConfig
	d, err := nilCfg.ParseMaxDuration()
	if err != nil {
		t.Fatal(err)
	}
	if d != 4*time.Hour {
		t.Errorf("nil config default = %v, want 4h", d)
	}

	empty := &CronJobConfig{}
	d, err = empty.ParseMaxDuration()
	if err != nil {
		t.Fatal(err)
	}
	if d != 4*time.Hour {
		t.Errorf("empty MaxDuration = %v, want 4h", d)
	}

	custom := &CronJobConfig{MaxDuration: "30m"}
	d, err = custom.ParseMaxDuration()
	if err != nil {
		t.Fatal(err)
	}
	if d != 30*time.Minute {
		t.Errorf("got %v, want 30m", d)
	}

	bad := &CronJobConfig{MaxDuration: "not-a-duration"}
	_, err = bad.ParseMaxDuration()
	if err == nil {
		t.Fatal("expected error for invalid max_duration")
	}
}
