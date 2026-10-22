package cliapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vkh/spacemosquito/internal/config"
	"github.com/vkh/spacemosquito/internal/paths"
	"github.com/vkh/spacemosquito/internal/updatecheck"
)

// updateChecker returns the release checker for this binary, or nil when the
// check is disabled (config update_check: false, SPACEMOSQUITO_NO_UPDATE_CHECK,
// development builds).
func updateChecker(cfg *config.Config) *updatecheck.Checker {
	if !cfg.UpdateCheckEnabled() {
		return nil
	}
	c := &updatecheck.Checker{Current: Version}
	if dir, err := paths.ResolveDataDir(); err == nil {
		c.CachePath = filepath.Join(dir, updatecheck.CacheFile)
	}
	if !c.Enabled() {
		return nil
	}
	return c
}

// checkForUpdate asks GitHub (at most once a day, a few seconds at most) and
// prints how to upgrade when a newer release exists. Errors stay silent.
func checkForUpdate(cfg *config.Config) {
	c := updateChecker(cfg)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if latest, newer, err := c.Latest(ctx); err == nil && newer {
		fmt.Print("\n" + updatecheck.Notice(Version, latest, executable()) + "\n")
	}
}

// printCachedUpdateNotice shows the result of the last check on stderr (so
// command output stays clean) without network access.
func printCachedUpdateNotice(cfg *config.Config) {
	c := updateChecker(cfg)
	if c == nil {
		return
	}
	if latest, newer := c.Cached(); newer {
		fmt.Fprint(os.Stderr, updatecheck.Notice(Version, latest, executable())+"\n")
	}
}

// loadConfigQuietly loads the config for commands that also work without one
// (version); nil when there is none.
func loadConfigQuietly() *config.Config {
	path, err := paths.ResolveConfig()
	if err != nil {
		return nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil
	}
	return cfg
}

func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}
