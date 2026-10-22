package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database DatabaseConfig `yaml:"database"`
	Storage  StorageConfig  `yaml:"storage"`
	Session  SessionConfig  `yaml:"session"`
	Browser  BrowserConfig  `yaml:"browser"`
	Embedder EmbedderConfig `yaml:"embedder"`
	MCP      MCPConfig      `yaml:"mcp"`
	Cron     CronConfig     `yaml:"cron"`
	// UpdateCheck: tell the user when a newer release is available (asks
	// GitHub at most once a day). Default on; false disables.
	UpdateCheck *bool `yaml:"update_check"`
}

// UpdateCheckEnabled reports whether the release update check may run.
func (c *Config) UpdateCheckEnabled() bool {
	return c == nil || c.UpdateCheck == nil || *c.UpdateCheck
}

// DatabaseConfig configures the embedded SQLite database.
type DatabaseConfig struct {
	Driver string `yaml:"driver"` // sqlite (default); postgres is rejected
	Path   string `yaml:"path"`   // sqlite database file (relative to data dir unless absolute)
}

// DriverName returns the configured driver, defaulting to sqlite.
func (c DatabaseConfig) DriverName() string {
	if c.Driver == "" {
		return "sqlite"
	}
	return c.Driver
}

type StorageConfig struct {
	BasePath string `yaml:"base_path"`
	// MaxAttachmentMB caps the size of page attachments (pptx, pdf, …) that
	// crawls download; larger files are linked to Confluence instead.
	// 0 = default (DefaultMaxAttachmentMB), -1 = no limit.
	MaxAttachmentMB int `yaml:"max_attachment_mb"`
	// AssetDelayMS is the pause before each image/attachment download, to stay
	// below Confluence's rate limits. 0 = default (DefaultAssetDelayMS),
	// -1 = no pause. Throttling (HTTP 429) still slows downloads down.
	AssetDelayMS int `yaml:"asset_delay_ms"`
}

// DefaultAssetDelayMS is the download pause when none is configured.
const DefaultAssetDelayMS = 2500

// AssetDelay returns the pause before each asset download.
func (s StorageConfig) AssetDelay() time.Duration {
	switch {
	case s.AssetDelayMS < 0:
		return 0
	case s.AssetDelayMS == 0:
		return DefaultAssetDelayMS * time.Millisecond
	}
	return time.Duration(s.AssetDelayMS) * time.Millisecond
}

// DefaultMaxAttachmentMB is the attachment size limit when none is configured.
const DefaultMaxAttachmentMB = 100

// MaxAttachmentBytes returns the attachment size limit in bytes; -1 = none.
func (s StorageConfig) MaxAttachmentBytes() int64 {
	switch {
	case s.MaxAttachmentMB < 0:
		return -1
	case s.MaxAttachmentMB == 0:
		return DefaultMaxAttachmentMB << 20
	}
	return int64(s.MaxAttachmentMB) << 20
}

type SessionConfig struct {
	EncryptionKey string `yaml:"encryption_key"`
	FilePath      string `yaml:"file_path"`
}

type BrowserConfig struct {
	Path string `yaml:"path"` // optional; CHROMIUM_PATH env takes precedence
}

type EmbedderConfig struct {
	Model  string         `yaml:"model"`
	OpenAI *OpenAICConfig `yaml:"openai"`
	BGE    *BGEConfig     `yaml:"bge"`
}

type OpenAICConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}

type BGEConfig struct {
	ModelPath string `yaml:"model_path"`
}

type MCPConfig struct {
	Port              int    `yaml:"port"`
	Host              string `yaml:"host"`
	Timeout           int    `yaml:"session_timeout"` // legacy; unused by Streamable HTTP MCP
	ExposeInternalIDs bool   `yaml:"expose_internal_ids"`
}

type CronConfig struct {
	FullCrawl   *CronJobConfig `yaml:"full_crawl"`
	Incremental *CronJobConfig `yaml:"incremental"`
}

type CronJobConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Interval    string   `yaml:"interval"`
	Spaces      []string `yaml:"spaces"`
	Detection   string   `yaml:"detection"`
	MaxDuration string   `yaml:"max_duration"`
}

func Load(path string) (*Config, error) {
	expanded := os.ExpandEnv(path)
	if expanded != path {
		p, err := filepath.Abs(expanded)
		if err == nil {
			path = p
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}
	if cfg.Storage.BasePath == "" {
		cfg.Storage.BasePath = "./saved"
	}
	if cfg.MCP.Port == 0 {
		cfg.MCP.Port = 56403
	}
	if cfg.MCP.Timeout == 0 {
		cfg.MCP.Timeout = 3600
	}
	if cfg.Embedder.Model == "" {
		cfg.Embedder.Model = "nomic-embed-text"
	}

	// Cron defaults
	if cfg.Cron.FullCrawl != nil && cfg.Cron.FullCrawl.Interval == "" {
		cfg.Cron.FullCrawl.Interval = "24h"
	}
	if cfg.Cron.Incremental != nil && cfg.Cron.Incremental.Interval == "" {
		cfg.Cron.Incremental.Interval = "2h"
	}
	if cfg.Cron.Incremental != nil && cfg.Cron.Incremental.Detection == "" {
		cfg.Cron.Incremental.Detection = "dom"
	}
	if cfg.Cron.FullCrawl != nil && cfg.Cron.FullCrawl.MaxDuration == "" {
		cfg.Cron.FullCrawl.MaxDuration = "4h"
	}
	if cfg.Cron.Incremental != nil && cfg.Cron.Incremental.MaxDuration == "" {
		cfg.Cron.Incremental.MaxDuration = "30m"
	}

	return &cfg, nil
}

// ParseCronDuration parses a cron interval string and returns the duration.
func ParseCronDuration(d string) (time.Duration, error) {
	return time.ParseDuration(d)
}

// ParseMaxDuration parses the MaxDuration string and returns a duration with a default fallback.
func (c *CronJobConfig) ParseMaxDuration() (time.Duration, error) {
	if c == nil || c.MaxDuration == "" {
		return 4 * time.Hour, nil
	}
	return time.ParseDuration(c.MaxDuration)
}
