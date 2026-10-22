package browse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vkh/spacemosquito/internal/catalogindex"
)

// ExportResult reports what Export wrote.
type ExportResult struct {
	Spaces int
	Pages  int
}

// Export writes the catalog (or one space when spaceKey is set) as static
// pages into outDir for browsing without the server (file://): catalog,
// space trees and pages with the sidebar, plus each page's assets. Content
// search needs the server and is left out. outDir must be empty or missing
// and outside the saved catalog.
func (s *Site) Export(ctx context.Context, outDir, spaceKey string) (ExportResult, error) {
	var res ExportResult
	if err := checkExportDir(outDir, s.SavedBase); err != nil {
		return res, err
	}
	spaces, err := catalogindex.LoadSpaces(ctx, s.DB, s.SavedBase)
	if err != nil {
		return res, err
	}
	if spaceKey != "" {
		var only []catalogindex.Space
		for _, sp := range spaces {
			if sp.Key == spaceKey {
				only = append(only, sp)
			}
		}
		if len(only) == 0 {
			return res, fmt.Errorf("space %s has no saved pages", spaceKey)
		}
		spaces = only
	}

	if err := os.MkdirAll(filepath.Join(outDir, staticDir), 0o755); err != nil {
		return res, err
	}
	for _, name := range []string{"browse.css", "browse.js"} {
		b, _ := staticFS.ReadFile("static/" + name)
		if err := os.WriteFile(filepath.Join(outDir, staticDir, name), b, 0o644); err != nil {
			return res, err
		}
	}
	if err := writeRendered(filepath.Join(outDir, "index.html"), func(w io.Writer) error {
		return renderRootList(w, spaces, true)
	}); err != nil {
		return res, err
	}

	for _, row := range spaces {
		sp, err := catalogindex.LoadSpace(ctx, s.DB, s.SavedBase, row.Key)
		if err != nil {
			return res, err
		}
		spaceOut := filepath.Join(outDir, sp.Key)
		if err := writeRendered(filepath.Join(spaceOut, "index.html"), func(w io.Writer) error {
			return renderSpaceTree(w, sp, true)
		}); err != nil {
			return res, err
		}
		var walk func([]*catalogindex.Node) error
		walk = func(ns []*catalogindex.Node) error {
			for _, n := range ns {
				if err := ctx.Err(); err != nil {
					return err
				}
				pageOut := filepath.Join(spaceOut, n.Dir)
				if err := writeRendered(filepath.Join(pageOut, "index.html"), func(w io.Writer) error {
					return renderPageView(w, s.SavedBase, sp, n.Dir, true)
				}); err != nil {
					return err
				}
				if err := copyTree(filepath.Join(s.SavedBase, sp.Key, n.Dir, "assets"), filepath.Join(pageOut, "assets")); err != nil {
					return fmt.Errorf("copy assets of %s: %w", n.Dir, err)
				}
				res.Pages++
				if err := walk(n.Children); err != nil {
					return err
				}
			}
			return nil
		}
		if err := walk(sp.Roots); err != nil {
			return res, err
		}
		res.Spaces++
	}
	return res, nil
}

// checkExportDir refuses targets inside the saved catalog (the export would
// mix generated pages back into it) and non-empty directories.
func checkExportDir(outDir, savedBase string) error {
	out, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if savedBase != "" {
		base, err := filepath.Abs(savedBase)
		if err != nil {
			return err
		}
		if rel, err := filepath.Rel(base, out); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("export directory %s is inside the saved catalog %s", out, base)
		}
	}
	entries, err := os.ReadDir(out)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("export directory %s is not empty", out)
	}
	return nil
}

func writeRendered(path string, render func(io.Writer) error) error {
	var b bytes.Buffer
	if err := render(&b); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// copyTree copies regular files under src to dst; a missing src is fine.
// Symlinks are skipped so an export cannot pull in files from elsewhere.
func copyTree(src, dst string) error {
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case !d.Type().IsRegular():
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
