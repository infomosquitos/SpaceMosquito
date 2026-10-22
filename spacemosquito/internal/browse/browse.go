// Package browse renders the saved catalog as web pages on request (ADR-018):
// the space list, each space's page tree with content search, and each page
// from its content.md with a page tree sidebar and quick search. Nothing is
// pre-generated, so UI changes apply to every page on restart. Export writes
// the same pages as static files for browsing without the server.
package browse

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vkh/spacemosquito/internal/catalogindex"
	"github.com/vkh/spacemosquito/internal/contentmd"
	"github.com/vkh/spacemosquito/internal/contentmd/csf"
	"github.com/vkh/spacemosquito/internal/spaceops"
	"github.com/vkh/spacemosquito/internal/storage"
	"github.com/vkh/spacemosquito/internal/store"
	"github.com/vkh/spacemosquito/pkg/logging"
)

// Prefix is the URL path under which the catalog is served.
const Prefix = "/browse/"

// staticDir holds browse.css and browse.js, under Prefix and in exports.
// Space keys cannot contain ".", so it never collides with a space.
const staticDir = ".static"

var (
	//go:embed templates.html
	templatesSrc string
	//go:embed static
	staticFS embed.FS

	tmpl = template.Must(template.New("browse").Parse(templatesSrc))
	// assetVersion busts browser caches of browse.css/browse.js on upgrade.
	assetVersion = func() string {
		h := sha256.New()
		for _, name := range []string{"browse.css", "browse.js"} {
			b, _ := staticFS.ReadFile("static/" + name)
			h.Write(b)
		}
		return hex.EncodeToString(h.Sum(nil))[:12]
	}()
)

// Site renders the catalog in savedBase using the page tree from db.
type Site struct {
	DB        store.Store
	SavedBase string
	Log       logging.Sugar
}

var errNotFound = errors.New("not found")

// Handler serves Prefix: catalog, space and page views, plus page files
// (assets/**, content.md, raw.html as plain text). No directory listings.
func (s *Site) Handler() http.Handler {
	return http.HandlerFunc(s.serve)
}

func (s *Site) serve(w http.ResponseWriter, r *http.Request) {
	rest, ok := strings.CutPrefix(r.URL.Path, Prefix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	dirPath := rest == "" || strings.HasSuffix(rest, "/")
	var segs []string
	if trimmed := strings.Trim(rest, "/"); trimmed != "" {
		segs = strings.Split(trimmed, "/")
	}
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			http.NotFound(w, r)
			return
		}
	}

	ctx := r.Context()
	var err error
	switch {
	case len(segs) == 0:
		err = s.writeHTML(w, func(b io.Writer) error { return s.renderRoot(ctx, b, false) })
	case segs[0] == staticDir:
		serveStatic(w, r, segs[1:])
		return
	case !spaceops.ValidKey(segs[0]):
		err = errNotFound
	case len(segs) == 1:
		if !dirPath {
			redirect(w, r, segs[0])
			return
		}
		err = s.writeHTML(w, func(b io.Writer) error { return s.renderSpace(ctx, b, segs[0], false) })
	case len(segs) == 2 && segs[1] == "index.html":
		redirect(w, r, segs[0])
		return
	case len(segs) == 2:
		if !dirPath {
			redirect(w, r, segs[0], segs[1])
			return
		}
		err = s.writeHTML(w, func(b io.Writer) error { return s.renderPage(ctx, b, segs[0], segs[1], false) })
	default:
		err = s.servePageFile(w, r, segs[0], segs[1], segs[2:])
	}
	switch {
	case err == nil:
	case errors.Is(err, errNotFound), errors.Is(err, catalogindex.ErrNoSavedDir):
		http.NotFound(w, r)
	default:
		s.Log.Errorw("browse render failed", "path", r.URL.Path, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// writeHTML renders into a buffer first so a failed render becomes a clean
// error response instead of a half-written page.
func (s *Site) writeHTML(w http.ResponseWriter, render func(io.Writer) error) error {
	var b bytes.Buffer
	if err := render(&b); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, err := w.Write(b.Bytes())
	return err
}

// redirect sends the client to the directory URL of segs (a space or page;
// old bookmarks to …/index.html and paths without the trailing slash).
// Segments are escaped: page directory names contain spaces and ">".
func redirect(w http.ResponseWriter, r *http.Request, segs ...string) {
	to := Prefix
	for _, seg := range segs {
		to += url.PathEscape(seg) + "/"
	}
	http.Redirect(w, r, to, http.StatusMovedPermanently)
}

func serveStatic(w http.ResponseWriter, r *http.Request, segs []string) {
	if len(segs) != 1 {
		http.NotFound(w, r)
		return
	}
	b, err := staticFS.ReadFile("static/" + segs[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctype := "text/css; charset=utf-8"
	if strings.HasSuffix(segs[0], ".js") {
		ctype = "text/javascript; charset=utf-8"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

// servePageFile serves content.md, raw.html (as plain text: Confluence
// source, never executed) and files under assets/. Responses are sandboxed so
// a saved attachment cannot run script against the local API.
func (s *Site) servePageFile(w http.ResponseWriter, r *http.Request, key, dir string, file []string) error {
	ctype := ""
	switch {
	case len(file) == 1 && file[0] == "index.html":
		redirect(w, r, key, dir)
		return nil
	case len(file) == 1 && file[0] == "content.md":
		ctype = "text/markdown; charset=utf-8"
	case len(file) == 1 && file[0] == "raw.html":
		ctype = "text/plain; charset=utf-8"
	case len(file) >= 2 && file[0] == "assets":
	default:
		return errNotFound
	}
	full := filepath.Join(append([]string{s.SavedBase, key, dir}, file...)...)
	f, err := os.Open(full)
	if err != nil {
		return errNotFound
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return errNotFound
	}
	// PDFs are shown inline by the browser's own viewer, which a sandboxed
	// response would block; it runs no script in this origin. Other
	// attachments (pptx, xlsx, …) download instead of rendering.
	isPDF := strings.EqualFold(path.Ext(st.Name()), ".pdf")
	if !isPDF {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	if len(file) >= 3 && file[1] == "attachments" && !isPDF {
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(st.Name()))
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	return nil
}

// Views. Links are relative to the rendered page, so the same HTML works
// under Prefix and as exported files; static (exported) pages link
// "…/index.html" and have no content search (it needs the server).

type common struct {
	DocTitle     string
	Base         string // relative path to the catalog root
	AssetVersion string
	Search       bool
}

type viewNode struct {
	Title    string
	Href     string
	Open     bool
	Current  bool
	Children []*viewNode
}

type spaceRow struct {
	Heading, Href, Summary string
}

type rootView struct {
	common
	Key    string // "": search all spaces
	Spaces []spaceRow
}

type spaceView struct {
	common
	Key, Heading, Summary, ConfluenceURL, RootHref string
	Tree                                           []*viewNode
}

type pageView struct {
	common
	Key, SpaceHeading, SpaceSummary, SpaceHref, RootHref string
	Title, SourceURL, Byline                             string
	Body                                                 template.HTML
	Children                                             []*viewNode
	Files                                                []fileView
	Tree                                                 []*viewNode
}

func indexFile(static bool) string {
	if static {
		return "index.html"
	}
	return ""
}

func (s *Site) renderRoot(ctx context.Context, w io.Writer, static bool) error {
	spaces, err := catalogindex.LoadSpaces(ctx, s.DB, s.SavedBase)
	if err != nil {
		return err
	}
	return renderRootList(w, spaces, static)
}

func renderRootList(w io.Writer, spaces []catalogindex.Space, static bool) error {
	v := rootView{common: common{DocTitle: "SpaceMosquito catalog", AssetVersion: assetVersion, Search: !static}}
	for i := range spaces {
		sp := &spaces[i]
		v.Spaces = append(v.Spaces, spaceRow{
			Heading: sp.Heading(),
			Href:    url.PathEscape(sp.Key) + "/" + indexFile(static),
			Summary: sp.Summary(),
		})
	}
	return tmpl.ExecuteTemplate(w, "root", v)
}

// loadSpace builds the page tree per request (the tree changes during
// crawls); errNotFound when the space is unknown.
func (s *Site) loadSpace(ctx context.Context, key string) (*catalogindex.Space, error) {
	if sp, err := s.DB.GetSpaceByKey(ctx, key); err != nil || sp == nil {
		return nil, errNotFound
	}
	return catalogindex.LoadSpace(ctx, s.DB, s.SavedBase, key)
}

func (s *Site) renderSpace(ctx context.Context, w io.Writer, key string, static bool) error {
	sp, err := s.loadSpace(ctx, key)
	if err != nil {
		return err
	}
	return renderSpaceTree(w, sp, static)
}

func renderSpaceTree(w io.Writer, sp *catalogindex.Space, static bool) error {
	file := indexFile(static)
	v := spaceView{
		common:        common{DocTitle: sp.Heading(), Base: "../", AssetVersion: assetVersion, Search: !static},
		Key:           sp.Key,
		Heading:       sp.Heading(),
		Summary:       sp.Summary(),
		ConfluenceURL: sp.URL,
		RootHref:      "../" + file,
		// First level open; deeper levels collapsed.
		Tree: viewTree(sp.Roots, "", file, "", func(depth int, _ *catalogindex.Node) bool { return depth == 0 }, 0),
	}
	return tmpl.ExecuteTemplate(w, "space", v)
}

func (s *Site) renderPage(ctx context.Context, w io.Writer, key, dir string, static bool) error {
	if st, err := os.Stat(filepath.Join(s.SavedBase, key, dir)); err != nil || !st.IsDir() {
		return errNotFound
	}
	sp, err := s.loadSpace(ctx, key)
	if err != nil {
		return err
	}
	return renderPageView(w, s.SavedBase, sp, dir, static)
}

func renderPageView(w io.Writer, savedBase string, sp *catalogindex.Space, dir string, static bool) error {
	file := indexFile(static)
	pageDir := filepath.Join(savedBase, sp.Key, dir)
	path := findPath(sp.Roots, dir)
	var node *catalogindex.Node
	if len(path) > 0 {
		node = path[len(path)-1]
	}
	open := map[*catalogindex.Node]bool{}
	for _, n := range path {
		open[n] = true
	}

	var meta struct {
		Title         string             `json:"title"`
		ConfluenceURL string             `json:"confluence_url"`
		Attachments   []storage.AssetRef `json:"attachments"`
	}
	if b, err := os.ReadFile(filepath.Join(pageDir, "metadata.json")); err == nil {
		_ = json.Unmarshal(b, &meta)
	}
	title := meta.Title
	if title == "" && node != nil {
		title = node.Title
	}
	if title == "" {
		title = dir
	}

	v := pageView{
		common:       common{DocTitle: title, Base: "../../", AssetVersion: assetVersion, Search: !static},
		Key:          sp.Key,
		SpaceHeading: sp.Heading(),
		SpaceSummary: sp.Summary(),
		SpaceHref:    "../" + file,
		RootHref:     "../../" + file,
		Title:        title,
		SourceURL:    meta.ConfluenceURL,
		Tree:         viewTree(sp.Roots, "../", file, dir, func(_ int, n *catalogindex.Node) bool { return open[n] }, 0),
	}

	if node != nil {
		v.Byline = byline(node.History)
	}
	v.Files = fileViews(meta.Attachments)

	md, err := os.ReadFile(filepath.Join(pageDir, contentmd.ContentFileName))
	switch {
	case err == nil && strings.TrimSpace(string(md)) != "":
		if v.Body, err = contentmd.RenderMarkdown(string(md)); err != nil {
			return err
		}
		v.Body = markCallouts(v.Body)
	case err != nil && !os.IsNotExist(err):
		return fmt.Errorf("read %s: %w", contentmd.ContentFileName, err)
	case node != nil:
		// Pages that only group other pages list their children, as
		// Confluence shows them.
		v.Children = viewTree(node.Children, "../", file, "", func(int, *catalogindex.Node) bool { return false }, 0)
		for _, c := range v.Children {
			c.Children = nil
		}
	}
	return tmpl.ExecuteTemplate(w, "page", v)
}

// calloutStart matches a Confluence panel as content.md renders it: a
// blockquote opening with its bold type label (see csf.calloutBlock).
var calloutStart = regexp.MustCompile(`<blockquote>\s*<p><strong>(Info|Note|Warning|Tip|Success|Error):</strong>`)

// markCallouts tags panel blockquotes with their type (sm-callout-warning, …)
// so the stylesheet can color them like Confluence does.
func markCallouts(body template.HTML) template.HTML {
	return template.HTML(calloutStart.ReplaceAllStringFunc(string(body), func(m string) string {
		label := calloutStart.FindStringSubmatch(m)[1]
		return strings.Replace(m, "<blockquote>", `<blockquote class="sm-callout-`+strings.ToLower(label)+`">`, 1)
	}))
}

// fileView is one row of a page's Attachments section.
type fileView struct {
	Name, Href, Size, Icon string
	Local, Download        bool // saved locally; offer as download (not PDF)
}

// fileViews lists a page's file attachments: the local copy when saved, else
// the Confluence link (over the size limit or the download failed).
func fileViews(refs []storage.AssetRef) []fileView {
	var out []fileView
	for _, r := range refs {
		name := r.Name
		if name == "" {
			name = path.Base(r.LocalPath)
		}
		f := fileView{Name: name, Href: r.OriginalURL, Icon: fileIcon(name)}
		if r.LocalPath != "" {
			segs := strings.Split(filepath.ToSlash(r.LocalPath), "/")
			for i := range segs {
				segs[i] = url.PathEscape(segs[i])
			}
			f.Href = strings.Join(segs, "/")
			f.Local = true
			f.Download = !strings.EqualFold(path.Ext(name), ".pdf")
		}
		if r.Size > 0 {
			f.Size = csf.FormatSize(r.Size)
		}
		if name == "" || name == "." || f.Href == "" {
			continue
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func fileIcon(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".pdf":
		return "📕"
	case ".ppt", ".pptx", ".key", ".odp":
		return "📙"
	case ".xls", ".xlsx", ".csv", ".ods":
		return "📗"
	case ".doc", ".docx", ".odt", ".rtf", ".txt", ".md":
		return "📘"
	case ".zip", ".7z", ".gz", ".tar", ".rar":
		return "🗜"
	case ".mp4", ".mov", ".webm", ".avi":
		return "🎬"
	}
	return "📄"
}

// byline describes who created and last updated a page in Confluence, e.g.
// "Created by Ann on 12 Mar 2022 · Last updated by Bob on 3 Oct 2024".
func byline(h store.PageHistory) string {
	part := func(verb, by string, at *time.Time) string {
		s := verb
		if by != "" {
			s += " by " + by
		}
		if at != nil {
			s += " on " + at.Local().Format("2 Jan 2006")
		}
		if s == verb {
			return ""
		}
		return s
	}
	var parts []string
	if p := part("Created", h.CreatedBy, h.CreatedAt); p != "" {
		parts = append(parts, p)
	}
	if p := part("Last updated", h.UpdatedBy, h.UpdatedAt); p != "" {
		parts = append(parts, p)
	}
	return strings.Join(parts, " · ")
}

// viewTree converts a page tree into view nodes linking prefix+dir/+file.
func viewTree(ns []*catalogindex.Node, prefix, file, current string, open func(int, *catalogindex.Node) bool, depth int) []*viewNode {
	out := make([]*viewNode, 0, len(ns))
	for _, n := range ns {
		out = append(out, &viewNode{
			Title:    n.Title,
			Href:     prefix + url.PathEscape(n.Dir) + "/" + file,
			Open:     len(n.Children) > 0 && open(depth, n),
			Current:  n.Dir == current,
			Children: viewTree(n.Children, prefix, file, current, open, depth+1),
		})
	}
	return out
}

// findPath returns the nodes from a root down to the page in dir, or nil.
func findPath(ns []*catalogindex.Node, dir string) []*catalogindex.Node {
	for _, n := range ns {
		if n.Dir == dir {
			return []*catalogindex.Node{n}
		}
		if p := findPath(n.Children, dir); p != nil {
			return append([]*catalogindex.Node{n}, p...)
		}
	}
	return nil
}
