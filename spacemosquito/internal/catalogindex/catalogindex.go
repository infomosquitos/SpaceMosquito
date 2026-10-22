// Package catalogindex writes navigation files into the saved catalog so it
// can be browsed offline: saved/index.{html,md} lists spaces, and
// saved/<SPACE>/index.{html,md} holds the space's page tree (rebuilt from
// parent_confluence_id in the DB), and saved/<SPACE>/tree.js renders the same
// tree as a sidebar on every page. Page links are relative, so the catalog
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

	"github.com/vkh/spacemosquito/internal/contentmd"
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
	Empty    bool   // Storage Format page without content of its own
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
	if err := writeTreeScript(spaceDir, tree); err != nil {
		return nil, err
	}
	if err := writeContainerPages(spaceDir, tree.Roots); err != nil {
		return nil, err
	}
	return tree, nil
}

// writeContainerPages gives empty parent pages (Confluence pages that only
// group other pages) an index.html listing their children, as Confluence
// shows them. Children change between crawls, so this runs with the index.
func writeContainerPages(spaceDir string, ns []*Node) error {
	for _, n := range ns {
		if n.Empty && len(n.Children) > 0 {
			var b strings.Builder
			b.WriteString("This page has no content of its own. Child pages:\n\n")
			for _, c := range n.Children {
				fmt.Fprintf(&b, "- [%s](../%s)\n", mdEscaper.Replace(c.Title), href(c.Dir, "index.html"))
			}
			if _, err := contentmd.RefreshDirIndexHTML(filepath.Join(spaceDir, n.Dir), b.String()); err != nil {
				return fmt.Errorf("write container page %s: %w", n.Dir, err)
			}
		}
		if err := writeContainerPages(spaceDir, n.Children); err != nil {
			return err
		}
	}
	return nil
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
		empty := p.BodyFormat == "storage" && strings.TrimSpace(p.Content) == ""
		nodes[p.ConfluenceID] = &Node{Title: title, Dir: dir, Empty: empty}
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
summary { cursor: pointer; list-style: none; }
summary::-webkit-details-marker { display: none; }
summary::before { content: "▶"; display: inline-block; width: 20px; height: 20px; line-height: 20px; margin-right: 2px; font-size: 10px; text-align: center; vertical-align: 1px; border-radius: 4px; opacity: .6; transition: transform .15s; }
details[open] > summary::before { transform: rotate(90deg); }
summary:hover::before { background: #8883; opacity: 1; }
li.leaf { padding-left: 22px; }
.count { font-size: 13px; opacity: .6; }
.tree-tools { display: flex; gap: 8px; align-items: center; margin: 8px 0 16px; }
.tree-tools input[type=search] { flex: 1; margin: 0; }
.tree-tools button { font: inherit; font-size: 13px; padding: 5px 10px; border: 1px solid #8886; border-radius: 4px; background: transparent; color: inherit; cursor: pointer; white-space: nowrap; }
.tree-tools button:hover { background: #8882; }
.hidden { display: none; }
#content-search { margin-bottom: 16px; }
#search-results { padding-left: 20px; margin: 0; }
#search-results li { margin-bottom: 8px; }
.excerpt { margin: 2px 0 0; font-size: 13px; opacity: .85; }
`

// searchSection is the full-text content search, called with the space key
// ("" searches all spaces). It needs the backend (/api/search, the same FTS
// as MCP), so it stays hidden when an index is opened as a file:// page.
// Results are built with DOM APIs (textContent), never innerHTML.
const searchSection = `{{define "search"}}<section id="content-search" class="hidden" data-space="{{.}}">
<input type="search" id="search" placeholder="Search page content{{if .}} in this space{{end}}…" aria-label="Search page content">
<p id="search-status" class="meta"></p>
<ol id="search-results"></ol>
</section>
<script>
(function () {
  if (!/^https?:$/.test(location.protocol)) return;
  var box = document.getElementById("content-search");
  var input = document.getElementById("search");
  var list = document.getElementById("search-results");
  var status = document.getElementById("search-status");
  var space = box.getAttribute("data-space");
  var timer = null, seq = 0;
  box.classList.remove("hidden");
  input.addEventListener("input", function () {
    clearTimeout(timer);
    timer = setTimeout(run, 250);
  });
  function run() {
    var q = input.value.trim(), my = ++seq;
    list.textContent = "";
    status.textContent = "";
    if (q.length < 2) return;
    var url = "/api/search?highlight=1&limit=20&q=" + encodeURIComponent(q) +
      (space ? "&space_key=" + encodeURIComponent(space) : "");
    fetch(url).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (data) {
      if (my !== seq) return;
      var hits = data.results || [];
      if (!hits.length) status.textContent = "No pages contain all of these words.";
      hits.forEach(function (h) {
        var li = document.createElement("li");
        var linked = typeof h.browse_url === "string" && h.browse_url.indexOf("/browse/") === 0;
        var title = document.createElement(linked ? "a" : "span");
        if (linked) title.href = h.browse_url;
        title.textContent = h.title;
        li.appendChild(title);
        if (!space) {
          var key = document.createElement("span");
          key.className = "meta";
          key.textContent = " · " + h.space_key;
          li.appendChild(key);
        }
        if (h.excerpt) {
          var p = document.createElement("p");
          p.className = "excerpt";
          appendMarked(p, h.excerpt);
          li.appendChild(p);
        }
        list.appendChild(li);
      });
    }).catch(function () {
      if (my === seq) status.textContent = "Content search is unavailable. Is spacemosquito serve running?";
    });
  }
  // Excerpts wrap matches in U+E000 … U+E001 (search.MatchStart/MatchEnd).
  function appendMarked(el, s) {
    s.split("\uE000").forEach(function (part, i) {
      var end = part.indexOf("\uE001");
      if (i === 0 || end < 0) {
        el.appendChild(document.createTextNode(part.replace(/\uE001/g, "")));
        return;
      }
      var m = document.createElement("mark");
      m.textContent = part.slice(0, end);
      el.appendChild(m);
      el.appendChild(document.createTextNode(part.slice(end + 1)));
    });
  }
})();
</script>{{end}}`

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
{{template "search" .Key}}
<div class="tree-tools">
<input type="search" id="filter" placeholder="Filter pages by title…" aria-label="Filter pages by title" autofocus>
<button type="button" id="expand-all" class="hidden" title="Show every page in the tree">Expand all</button>
<button type="button" id="collapse-all" class="hidden" title="Show only top-level pages">Collapse all</button>
</div>
<ul class="tree" id="tree">
{{template "nodes" .Roots}}
</ul>
<script>
(function () {
  var input = document.getElementById("filter");
  // Each item keeps the titles of itself and its descendants (not the counts).
  var items = Array.prototype.slice.call(document.querySelectorAll("#tree li")).map(function (li) {
    var titles = Array.prototype.map.call(li.querySelectorAll("a"), function (a) { return a.textContent; });
    return { li: li, text: titles.join("\n").toLowerCase() };
  });
  var folders = Array.prototype.slice.call(document.querySelectorAll("#tree details"));
  // Default view: top-level pages open, deeper levels collapsed.
  function reset() {
    folders.forEach(function (d) { d.open = d.parentElement.parentElement.id === "tree"; });
  }
  function setAll(open) {
    folders.forEach(function (d) { d.open = open; });
  }
  if (folders.length > 0) {
    ["expand-all", "collapse-all"].forEach(function (id, i) {
      var b = document.getElementById(id);
      b.classList.remove("hidden");
      b.addEventListener("click", function () { setAll(i === 0); });
    });
  }
  reset();
  input.addEventListener("input", function () {
    var q = input.value.trim().toLowerCase();
    items.forEach(function (it) {
      it.li.classList.toggle("hidden", q !== "" && it.text.indexOf(q) < 0);
    });
    if (q !== "") setAll(true); else reset();
  });
})();
</script>
</body>
</html>
{{define "nodes"}}{{range .}}<li{{if not .Children}} class="leaf"{{end}}>{{if .Children}}<details><summary><a href="{{href .Dir "index.html"}}">{{.Title}}</a> <span class="count">({{len .Children}})</span></summary><ul>
{{template "nodes" .Children}}</ul></details>{{else}}<a href="{{href .Dir "index.html"}}">{{.Title}}</a>{{end}}</li>
{{end}}{{end}}
` + searchSection))

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
{{if .}}{{template "search" ""}}
<ul>
{{range .}}<li><a href="{{href .Key "index.html"}}">{{heading .Key .Name}}</a> <span class="meta">— {{.Pages}} pages{{crawled .LastCrawled}}</span></li>
{{end}}</ul>{{else}}<p>No spaces saved yet.</p>{{end}}
</body>
</html>
` + searchSection))
