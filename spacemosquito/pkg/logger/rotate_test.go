package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "serve.log")
	r, err := OpenRotatingFile(path, 20, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"aaaaaaaaaa\n", "bbbbbbbbbb\n", "cccccccccc\n", "dddddddddd\n"} {
		if _, err := r.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	r.Close()
	read := func(p string) string {
		b, _ := os.ReadFile(p)
		return string(b)
	}
	// Each write would exceed 20 bytes, so every line starts a new file; two
	// old files are kept, the oldest dropped.
	if read(path) != "dddddddddd\n" || read(path+".1") != "cccccccccc\n" || read(path+".2") != "bbbbbbbbbb\n" {
		t.Fatalf("files: %q %q %q", read(path), read(path+".1"), read(path+".2"))
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("more files kept than configured")
	}
}

func TestNewProduction_alsoWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.log")
	log, err := NewProduction(&Config{Level: "info", Format: "json", File: path})
	if err != nil {
		t.Fatal(err)
	}
	log.Info("crawl started")
	_ = log.Sync()
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), `"msg":"crawl started"`) {
		t.Fatalf("log file: %q", b)
	}
}
