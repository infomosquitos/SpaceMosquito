package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Level  string `yaml:"level"`
	Format string // "json" or "console"
	// File, when set, also writes JSON log lines to this file, rotated by
	// size (FileMaxBytes, keeping FileKeep old files).
	File         string
	FileMaxBytes int64
	FileKeep     int
}

// Defaults for the log file rotation.
const (
	DefaultFileMaxBytes = 10 << 20
	DefaultFileKeep     = 3
)

func DefaultConfig() *Config {
	return &Config{
		Level:  "info",
		Format: "json",
	}
}

func NewProduction(cfg *Config) (*zap.Logger, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	var enc zapcore.Encoder

	if cfg.Format == "console" {
		enc = zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		})
	} else {
		enc = zapcore.NewJSONEncoder(zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.RFC3339TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		})
	}

	core := zapcore.NewCore(enc, zapcore.AddSync(os.Stdout), level)
	if cfg.File != "" {
		maxBytes, keep := cfg.FileMaxBytes, cfg.FileKeep
		if maxBytes <= 0 {
			maxBytes = DefaultFileMaxBytes
		}
		if keep <= 0 {
			keep = DefaultFileKeep
		}
		f, err := OpenRotatingFile(cfg.File, maxBytes, keep)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		fileEnc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
			TimeKey: "ts", LevelKey: "level", NameKey: "logger", CallerKey: "caller", MessageKey: "msg",
			StacktraceKey: "stacktrace", LineEnding: zapcore.DefaultLineEnding,
			EncodeLevel: zapcore.LowercaseLevelEncoder, EncodeTime: zapcore.RFC3339TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder, EncodeCaller: zapcore.ShortCallerEncoder,
		})
		core = zapcore.NewTee(core, zapcore.NewCore(fileEnc, zapcore.AddSync(f), level))
	}
	return zap.New(core), nil
}

func NewProductionWithLevel(level string) (*zap.Logger, error) {
	return NewProduction(&Config{Level: level, Format: "json"})
}
