// Package catalogindex builds a space's page tree from the DB
// (parent_confluence_id) and writes the Markdown navigation files for agents
// and Markdown viewers: saved/index.md lists spaces, saved/<SPACE>/index.md
// holds the page tree linking each page's content.md. The browse UI renders
// the same tree on request (internal/browse, ADR-018). Links are relative, so
// the catalog stays usable when moved.
package catalogindex

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vkh/spacemosquito/internal/contentmd"
	"github.com/vkh/spacemosquito/internal/store"
)

const (
	mdFile   = "index.md"
	pageSize = 500
)

// ErrNoSavedDir is returned by LoadSpace for a space without saved/<SPACE>.
var ErrNoSavedDir = errors.New("space has no saved directory")

// FolderDirPrefix starts the Dir of a folder node: folders have no saved
// directory; browse serves them as FolderDirPrefix+ID.
const FolderDirPrefix = "folder-"

// Node is one saved page (or a Confluence folder) in a space tree.
type Node struct {
	Title    string
	Dir      string // page directory name under the space directory; folder-<id> for folders
	Folder   bool   // Confluence Cloud folder: no content, holds pages
	Empty    bool   // Storage Format page without content of its own
	History  store.PageHistory
	Children []*Node
}

// Space is a space's page tree plus the summary shown in catalog listings.
type Space struct {
	Key         string
	Name        string
	URL         string
	LastCrawled *time.Time
	Pages       int
	Roots       []*Node
}

// Heading is the space's display name, e.g. "Commerce (SCCPI)".
func (s *Space) Heading() string { return spaceHeading(s.Key, s.Name) }

// Summary is e.g. "380 pages, last crawled 2026-10-22 15:19".
func (s *Space) Summary() string {
	return fmt.Sprintf("%d pages%s", s.Pages, crawledSuffix(s.LastCrawled))
}

// LoadSpace builds the page tree of a space that has a saved directory.
func LoadSpace(ctx context.Context, db store.Store, savedBase, spaceKey string) (*Space, error) {
	sp, err := db.GetSpaceByKey(ctx, spaceKey)
	if err != nil || sp == nil {
		return nil, fmt.Errorf("get space %s: %w", spaceKey, err)
	}
	return loadSpace(ctx, db, savedBase, *sp)
}

func loadSpace(ctx context.Context, db store.Store, savedBase string, sp store.Space) (*Space, error) {
	spaceDir := filepath.Join(savedBase, sp.Key)
	if !isDir(spaceDir) {
		return nil, ErrNoSavedDir
	}
	pages, err := listPages(ctx, db, sp.Key)
	if err != nil {
		return nil, err
	}
	folders, err := db.ListFolders(ctx, sp.Key)
	if err != nil {
		return nil, err
	}
	tree := &Space{Key: sp.Key, Name: sp.Name, URL: sp.URL, LastCrawled: sp.LastCrawled}
	tree.Roots, tree.Pages = BuildTree(pages, folders, spaceDir)
	return tree, nil
}

// LoadSpaces lists spaces that have a saved directory, with page counts (no
// trees), sorted by key.
func LoadSpaces(ctx context.Context, db store.Store, savedBase string) ([]Space, error) {
	spaces, err := db.ListSpaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list spaces: %w", err)
	}
	var list []Space
	for _, sp := range spaces {
		if !isDir(filepath.Join(savedBase, sp.Key)) {
			continue
		}
		n, err := db.CountPagesBySpaceID(ctx, sp.ID)
		if err != nil {
			return nil, fmt.Errorf("count pages %s: %w", sp.Key, err)
		}
		list = append(list, Space{Key: sp.Key, Name: sp.Name, URL: sp.URL, LastCrawled: sp.LastCrawled, Pages: n})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	return list, nil
}

// WriteSpace rebuilds saved/<spaceKey>/index.md and the root index.md.
func WriteSpace(ctx context.Context, db store.Store, savedBase, spaceKey string) error {
	sp, err := db.GetSpaceByKey(ctx, spaceKey)
	if err != nil {
		return fmt.Errorf("get space %s: %w", spaceKey, err)
	}
	if err := writeSpace(ctx, db, savedBase, *sp); err != nil {
		return err
	}
	return WriteRoot(ctx, db, savedBase)
}

// WriteAll rebuilds the index.md of every space and the root index.md.
func WriteAll(ctx context.Context, db store.Store, savedBase string) error {
	spaces, err := db.ListSpaces(ctx)
	if err != nil {
		return fmt.Errorf("list spaces: %w", err)
	}
	for _, sp := range spaces {
		if err := writeSpace(ctx, db, savedBase, sp); err != nil {
			return err
		}
	}
	return WriteRoot(ctx, db, savedBase)
}

// WriteRoot rebuilds saved/index.md, listing spaces that have a saved
// directory.
func WriteRoot(ctx context.Context, db store.Store, savedBase string) error {
	list, err := LoadSpaces(ctx, db, savedBase)
	if err != nil {
		return err
	}
	return writeMarkdown(savedBase, renderRootMarkdown(list))
}

func writeSpace(ctx context.Context, db store.Store, savedBase string, sp store.Space) error {
	tree, err := loadSpace(ctx, db, savedBase, sp)
	if errors.Is(err, ErrNoSavedDir) {
		return nil
	}
	if err != nil {
		return err
	}
	return writeMarkdown(filepath.Join(savedBase, sp.Key), renderSpaceMarkdown(tree))
}

// RemoveLegacyHTML deletes HTML that versions before ADR-018 generated into
// the saved catalog: saved/index.html, saved/<SPACE>/index.html and tree.js,
// and index.html in Storage Format page directories (rendered from
// content.md; their source is raw.html). Browser-fallback pages keep their
// index.html, the source of their content.md. Returns the number of files
// removed.
func RemoveLegacyHTML(savedBase string) (int, error) {
	removed := 0
	rm := func(path string) error {
		err := os.Remove(path)
		if err == nil {
			removed++
			return nil
		}
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := rm(filepath.Join(savedBase, "index.html")); err != nil {
		return removed, err
	}
	spaces, err := os.ReadDir(savedBase)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return removed, err
	}
	for _, sp := range spaces {
		if !sp.IsDir() {
			continue
		}
		spaceDir := filepath.Join(savedBase, sp.Name())
		for _, f := range []string{"index.html", "tree.js", "tree.js.tmp"} {
			if err := rm(filepath.Join(spaceDir, f)); err != nil {
				return removed, err
			}
		}
		pages, err := os.ReadDir(spaceDir)
		if err != nil {
			return removed, err
		}
		for _, pg := range pages {
			dir := filepath.Join(spaceDir, pg.Name())
			if !pg.IsDir() || contentmd.DetectBodyFormat(dir) != "storage" {
				continue
			}
			if err := rm(filepath.Join(dir, "index.html")); err != nil {
				return removed, err
			}
		}
	}
	return removed, nil
}

func listPages(ctx context.Context, db store.Store, spaceKey string) ([]store.Page, error) {
	var all []store.Page
	var after *int
	for {
		batch, err := db.ListPages(ctx, spaceKey, pageSize, after)
		if err != nil {
			return nil, fmt.Errorf("list pages %s: %w", spaceKey, err)
		}
		all = append(all, batch...)
		if len(batch) < pageSize {
			return all, nil
		}
		last := batch[len(batch)-1].ConfluenceID
		after = &last
	}
}

// BuildTree arranges pages saved under spaceDir by parent, sorting siblings
// by title. Folders become nodes too (kept only when they hold pages); pages
// whose parent is neither saved nor a folder become roots. Returns the roots
// and the number of pages placed.
func BuildTree(pages []store.Page, folders []store.Folder, spaceDir string) ([]*Node, int) {
	nodes := map[int]*Node{}
	var order []int
	pageCount := 0
	for _, p := range pages {
		dir := pageDirName(p, spaceDir)
		if dir == "" {
			continue
		}
		title := p.Title
		if title == "" {
			title = dir
		}
		empty := p.BodyFormat == "storage" && strings.TrimSpace(p.Content) == ""
		nodes[p.ConfluenceID] = &Node{Title: title, Dir: dir, Empty: empty, History: p.History}
		order = append(order, p.ConfluenceID)
		pageCount++
	}
	parents := map[int]int{}
	for _, p := range pages {
		if p.ParentConfluenceID != nil {
			parents[p.ConfluenceID] = *p.ParentConfluenceID
		}
	}
	for _, f := range folders {
		if nodes[f.ConfluenceID] != nil {
			continue
		}
		title := f.Title
		if title == "" {
			title = "Folder"
		}
		nodes[f.ConfluenceID] = &Node{Title: title, Dir: FolderDirPrefix + strconv.Itoa(f.ConfluenceID), Folder: true}
		order = append(order, f.ConfluenceID)
		if f.ParentConfluenceID != nil {
			parents[f.ConfluenceID] = *f.ParentConfluenceID
		}
	}

	var roots []*Node
	for _, id := range order {
		n := nodes[id]
		if pid, ok := parents[id]; ok && nodes[pid] != nil && !createsCycle(id, parents, nodes) {
			nodes[pid].Children = append(nodes[pid].Children, n)
			continue
		}
		roots = append(roots, n)
	}
	roots = pruneEmptyFolders(roots)
	sortNodes(roots)
	return roots, pageCount
}

// pruneEmptyFolders drops folders that hold no saved page (directly or in
// sub-folders).
func pruneEmptyFolders(ns []*Node) []*Node {
	out := ns[:0]
	for _, n := range ns {
		n.Children = pruneEmptyFolders(n.Children)
		if n.Folder && len(n.Children) == 0 {
			continue
		}
		out = append(out, n)
	}
	return out
}

// createsCycle reports whether following saved parents from id returns to id.
func createsCycle(id int, parents map[int]int, nodes map[int]*Node) bool {
	seen := map[int]bool{id: true}
	for cur := id; ; {
		pid, ok := parents[cur]
		if !ok || nodes[pid] == nil {
			return false
		}
		if seen[pid] {
			return true
		}
		seen[pid] = true
		cur = pid
	}
}

func sortNodes(ns []*Node) {
	sort.SliceStable(ns, func(i, j int) bool {
		return strings.ToLower(ns[i].Title) < strings.ToLower(ns[j].Title)
	})
	for _, n := range ns {
		sortNodes(n.Children)
	}
}

// pageDirName returns the page's directory name if it exists under spaceDir.
// The recorded FileDir may be absolute from another machine, so only its base
// name is trusted.
func pageDirName(p store.Page, spaceDir string) string {
	if p.FileDir == "" {
		return ""
	}
	name := filepath.Base(p.FileDir)
	if !isDir(filepath.Join(spaceDir, name)) {
		return ""
	}
	return name
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func writeMarkdown(dir, md string) error {
	if err := os.WriteFile(filepath.Join(dir, mdFile), []byte(md), 0o644); err != nil {
		return fmt.Errorf("write index markdown: %w", err)
	}
	return nil
}

// href builds a relative link to file inside a page or space directory;
// directory names contain spaces and characters like ">".
func href(dir, file string) string {
	return url.PathEscape(dir) + "/" + file
}

var mdEscaper = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, `*`, `\*`, `_`, `\_`, "`", "\\`", `<`, `\<`)

func renderSpaceMarkdown(sp *Space) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", mdEscaper.Replace(spaceHeading(sp.Key, sp.Name)))
	fmt.Fprintf(&b, "[All spaces](../%s)", mdFile)
	if sp.URL != "" {
		fmt.Fprintf(&b, " · [Open in Confluence](<%s>)", sp.URL)
	}
	fmt.Fprintf(&b, "\n\n%d pages%s\n\n", sp.Pages, crawledSuffix(sp.LastCrawled))
	var walk func(ns []*Node, depth int)
	walk = func(ns []*Node, depth int) {
		for _, n := range ns {
			if n.Folder {
				fmt.Fprintf(&b, "%s- 📁 %s\n", strings.Repeat("  ", depth), mdEscaper.Replace(n.Title))
			} else {
				fmt.Fprintf(&b, "%s- [%s](%s)\n", strings.Repeat("  ", depth), mdEscaper.Replace(n.Title), href(n.Dir, "content.md"))
			}
			walk(n.Children, depth+1)
		}
	}
	walk(sp.Roots, 0)
	return b.String()
}

func renderRootMarkdown(spaces []Space) string {
	var b strings.Builder
	b.WriteString("# SpaceMosquito catalog\n\n")
	if len(spaces) == 0 {
		b.WriteString("No spaces saved yet.\n")
	}
	for _, sp := range spaces {
		fmt.Fprintf(&b, "- [%s](%s) — %d pages%s\n", mdEscaper.Replace(spaceHeading(sp.Key, sp.Name)), href(sp.Key, mdFile), sp.Pages, crawledSuffix(sp.LastCrawled))
	}
	return b.String()
}

func spaceHeading(key, name string) string {
	if name == "" || name == key {
		return key
	}
	return name + " (" + key + ")"
}

func crawledSuffix(t *time.Time) string {
	if t == nil {
		return ""
	}
	return ", last crawled " + t.Local().Format("2006-01-02 15:04")
}
