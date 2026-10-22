// Package catalogindex writes navigation files into the saved catalog so it
// can be browsed offline: saved/index.{html,md} lists spaces, and
// saved/<SPACE>/index.{html,md} holds the space's page tree (rebuilt from
// parent_confluence_id in the DB). Page links are relative, so the catalog
// stays browsable when moved.
package catalogindex

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vkh/spacemosquito/internal/store"
)

const (
	htmlFile = "index.html"
	mdFile   = "index.md"
	pageSize = 500
)

// Node is one saved page in a space tree.
type Node struct {
	Title    string
	Dir      string // page directory name under the space directory
	Children []*Node
}

// Space is a space's page tree plus the summary shown in the root index.
type Space struct {
	Key         string
	Name        string
	URL         string
	LastCrawled *time.Time
	Pages       int
	Roots       []*Node
}

// WriteSpace rebuilds saved/<spaceKey>/index.{html,md} and the root index.
func WriteSpace(ctx context.Context, db store.Store, savedBase, spaceKey string) error {
	sp, err := db.GetSpaceByKey(ctx, spaceKey)
	if err != nil {
		return fmt.Errorf("get space %s: %w", spaceKey, err)
	}
	if _, err := writeSpace(ctx, db, savedBase, *sp); err != nil {
		return err
	}
	return WriteRoot(ctx, db, savedBase)
}

// WriteAll rebuilds the index files of every space and the root index.
func WriteAll(ctx context.Context, db store.Store, savedBase string) error {
	spaces, err := db.ListSpaces(ctx)
	if err != nil {
		return fmt.Errorf("list spaces: %w", err)
	}
	for _, sp := range spaces {
		if _, err := writeSpace(ctx, db, savedBase, sp); err != nil {
			return err
		}
	}
	return WriteRoot(ctx, db, savedBase)
}

// WriteRoot rebuilds saved/index.{html,md}, listing spaces that have a saved
// directory.
func WriteRoot(ctx context.Context, db store.Store, savedBase string) error {
	spaces, err := db.ListSpaces(ctx)
	if err != nil {
		return fmt.Errorf("list spaces: %w", err)
	}
	var list []Space
	for _, sp := range spaces {
		if !isDir(filepath.Join(savedBase, sp.Key)) {
			continue
		}
		n, err := db.CountPagesBySpaceID(ctx, sp.ID)
		if err != nil {
			return fmt.Errorf("count pages %s: %w", sp.Key, err)
		}
		list = append(list, Space{Key: sp.Key, Name: sp.Name, URL: sp.URL, LastCrawled: sp.LastCrawled, Pages: n})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	return writePair(savedBase, rootTemplate, list, renderRootMarkdown(list))
}

func writeSpace(ctx context.Context, db store.Store, savedBase string, sp store.Space) (*Space, error) {
	spaceDir := filepath.Join(savedBase, sp.Key)
	if !isDir(spaceDir) {
		return nil, nil
	}
	pages, err := listPages(ctx, db, sp.Key)
	if err != nil {
		return nil, err
	}
	tree := &Space{Key: sp.Key, Name: sp.Name, URL: sp.URL, LastCrawled: sp.LastCrawled}
	tree.Roots, tree.Pages = BuildTree(pages, spaceDir)
	if err := writePair(spaceDir, spaceTemplate, tree, renderSpaceMarkdown(tree)); err != nil {
		return nil, err
	}
	return tree, nil
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
// by title. Pages whose parent was not saved become roots. Returns the roots
// and the number of pages placed.
func BuildTree(pages []store.Page, spaceDir string) ([]*Node, int) {
	nodes := map[int]*Node{}
	var order []int
	for _, p := range pages {
		dir := pageDirName(p, spaceDir)
		if dir == "" {
			continue
		}
		title := p.Title
		if title == "" {
			title = dir
		}
		nodes[p.ConfluenceID] = &Node{Title: title, Dir: dir}
		order = append(order, p.ConfluenceID)
	}
	parents := map[int]int{}
	for _, p := range pages {
		if p.ParentConfluenceID != nil {
			parents[p.ConfluenceID] = *p.ParentConfluenceID
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
	sortNodes(roots)
	return roots, len(nodes)
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

func writePair(dir string, tmpl *template.Template, data any, md string) error {
	var b bytes.Buffer
	if err := tmpl.Execute(&b, data); err != nil {
		return fmt.Errorf("render %s: %w", filepath.Join(dir, htmlFile), err)
	}
	if err := os.WriteFile(filepath.Join(dir, htmlFile), b.Bytes(), 0644); err != nil {
		return fmt.Errorf("write index html: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, mdFile), []byte(md), 0644); err != nil {
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
			fmt.Fprintf(&b, "%s- [%s](%s)\n", strings.Repeat("  ", depth), mdEscaper.Replace(n.Title), href(n.Dir, "content.md"))
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

var funcs = template.FuncMap{
	"href":    func(dir, file string) template.URL { return template.URL(href(dir, file)) },
	"heading": spaceHeading,
	"crawled": crawledSuffix,
}

const styles = `
:root { color-scheme: light dark; }
body { max-width: 960px; margin: 0 auto; padding: 24px 16px; font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
header { border-bottom: 1px solid #8884; margin-bottom: 16px; }
nav, .meta { font-size: 13px; }
input[type=search] { width: 100%; box-sizing: border-box; padding: 6px 10px; margin: 8px 0 16px; font: inherit; border: 1px solid #8886; border-radius: 4px; background: transparent; }
ul.tree, ul.tree ul { list-style: none; padding-left: 18px; margin: 0; }
ul.tree { padding-left: 0; }
ul.tree li { margin: 2px 0; }
summary { cursor: pointer; }
li.leaf { padding-left: 1.1em; }
.hidden { display: none; }
`

var spaceTemplate = template.Must(template.New("space").Funcs(funcs).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{heading .Key .Name}}</title>
<style>` + styles + `</style>
</head>
<body>
<header>
<nav><a href="../index.html">← All spaces</a>{{if .URL}} · <a href="{{.URL}}">Open in Confluence</a>{{end}}</nav>
<h1>{{heading .Key .Name}}</h1>
<p class="meta">{{.Pages}} pages{{crawled .LastCrawled}}</p>
</header>
<input type="search" id="filter" placeholder="Filter pages by title…" autofocus>
<ul class="tree" id="tree">
{{template "nodes" .Roots}}
</ul>
<script>
(function () {
  var input = document.getElementById("filter");
  var items = Array.prototype.slice.call(document.querySelectorAll("#tree li"));
  input.addEventListener("input", function () {
    var q = input.value.trim().toLowerCase();
    items.forEach(function (li) {
      li.classList.toggle("hidden", q !== "" && li.textContent.toLowerCase().indexOf(q) < 0);
      var d = li.querySelector(":scope > details");
      if (d) d.open = q !== "";
    });
  });
})();
</script>
</body>
</html>
{{define "nodes"}}{{range .}}<li{{if not .Children}} class="leaf"{{end}}>{{if .Children}}<details><summary><a href="{{href .Dir "index.html"}}">{{.Title}}</a></summary><ul>
{{template "nodes" .Children}}</ul></details>{{else}}<a href="{{href .Dir "index.html"}}">{{.Title}}</a>{{end}}</li>
{{end}}{{end}}
`))

var rootTemplate = template.Must(template.New("root").Funcs(funcs).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>SpaceMosquito catalog</title>
<style>` + styles + `</style>
</head>
<body>
<header>
<h1>SpaceMosquito catalog</h1>
</header>
{{if .}}<ul>
{{range .}}<li><a href="{{href .Key "index.html"}}">{{heading .Key .Name}}</a> <span class="meta">— {{.Pages}} pages{{crawled .LastCrawled}}</span></li>
{{end}}</ul>{{else}}<p>No spaces saved yet.</p>{{end}}
</body>
</html>
`))
