# Development

How we run this project: ground rules, setup, cleanup, release, and product-specific notes.

## Ground rules

- Breaking changes in API are acceptable.
- When writing documentation use Mermaid for diagrams.
- When cleaning finished or cancelled work, follow [`CLEANUP.md`](CLEANUP.md). When cutting a version, follow [`RELEASE.md`](RELEASE.md).

## What's in this folder

| Doc | Role | Lifetime |
|-----|------|----------|
| [`README.md`](README.md) (this file) | Setup, ground rules, product notes | Durable |
| [`CLEANUP.md`](CLEANUP.md) | Promote durable content; remove finished tasks | Durable |
| [`RELEASE.md`](RELEASE.md) | Version changelog, bump extensions, tag / GitHub Release | Durable |
| [`BACKLOG.md`](BACKLOG.md) | Flat task index | Living — cleaned via CLEANUP.md |
| [`STATUS.md`](STATUS.md) | Current focus / blockers / session notes | Living — refreshed each session |
| [`tasks/`](tasks/) | Optional detail for a backlog item | Temporary — deleted on cleanup |

## Local Development & Build

```sh
cd spacemosquito
go build ./cmd/server
go build ./cmd/cli
go build ./cmd/spacemosquito
```

Release binaries (cross-compile, embedded SQLite migrations):

```sh
cd spacemosquito
./scripts/build-release.sh v0.2.0
ls dist/
```

## Browser extensions build

Each extension (`firefox-extension/`, `chrome-extension/`) builds on its own — config is duplicated, no shared build package:

```
npm run build  →  tsc --noEmit (TypeScript 7, typecheck only)  &&  node esbuild.mjs (bundles background + popup)
```

Node `>=16.20.0` (`engines`); CI uses Node 22. Dependabot may bump TypeScript majors — the `Extension Build` CI workflow is the gate.

## Run unit tests

```sh
cd spacemosquito
go test ./...
```

With the race detector (same as CI):

```sh
cd spacemosquito
go test -race ./...
```

## Integration tests (REST + MCP, in-process)

Requires the `integration` build tag. Boots a real SQLite DB with embedded migrations, seeds fixtures, and exercises HTTP + MCP Streamable HTTP.

```sh
cd spacemosquito
go test -race -tags=integration ./internal/app/...
```

Not run in CI by default.

## Search

- Multi-word queries use **AND** — all terms must appear in title or body.
- **Title is weighted 10×** over body in BM25 ranking.
- Default result limit is **10** (CLI: `--limit N`; REST: `?limit=N`; MCP: `limit` field).
- If FTS returns no rows, search falls back to case-insensitive **title substring** match.

## Page content (Markdown)

Crawls and imports store page body text as **Markdown** (`content.md` on disk, `pages.content` in the DB) using HTML→Markdown conversion — not flat `doc.Text()` extraction. This preserves paragraph boundaries and improves search/MCP readability.

```
index.html  →  contentmd.HTMLToMarkdown()  →  content.md + pages.content  →  FTS
```

API crawls (Confluence Storage Format) go the other way for `index.html`:

```
raw.html (CSF)  →  csf.CSFToMarkdown()  →  content.md  →  contentmd.MarkdownToHTML()  →  index.html
```

Catalog navigation (`saved/index.{html,md}`, `saved/<SPACE>/index.{html,md}`) is rebuilt from the DB page tree by `catalogindex` at the end of `crawl` / `crawl-page` and in `reindex --content`.

`serve` exposes the catalog read-only at `GET /browse/` (no directory listings; folders only via their `index.html`). `GET /api/spaces` reports `browse_path` (e.g. `/browse/KEY/`) once a space index exists; the extension's Spaces tab shows a 📖 button for it. Browsers do not let extensions open `file://` pages, hence the HTTP route.

Removing a space (`spaceops.Delete`, used by `DELETE /api/spaces/{key}[?delete_files=1]`, the `delete-space` CLI and the extension 🗑 button) drops the space row, its pages and FTS entries (DB cascade + trigger), its per-space cron config, and its root-index entry; `saved/<KEY>` is deleted only on request. Keys must match `^~?[A-Za-z0-9_-]+$` so the folder can never resolve outside `saved/`. The REST delete returns 409 while that space is being crawled; the CLI cannot see `serve`'s crawls.

Served index pages also show a content search box calling `GET /api/search` (hidden for `file://`). REST search adds `browse_url` per hit and, with `highlight=1`, keeps match markers U+E000/U+E001 in `excerpt` (`search.MatchStart`/`MatchEnd`; stripped by default, so MCP/CLI output is unchanged). The page renders results with DOM `textContent` only.

Regenerate existing catalogs after upgrade:

```sh
./spacemosquito reindex --content
```

## Testing with curl

MCP is `POST /mcp` with a JSON body (not a long-lived stream). Example:

```sh
curl -s -X POST http://localhost:56403/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
```

Get a page by Confluence ID (REST):

```sh
curl -s http://localhost:56403/api/pages/42
curl -s "http://localhost:56403/api/pages/42?space_key=TST"
```

## Dependency vulnerabilities

Use **GitHub Dependabot** for:

- **Alerts** + **security-update PRs** (enable in Settings → Code security and analysis)
- **Monthly version bumps** — [`.github/dependabot.yml`](../.github/dependabot.yml) (gomod under `/spacemosquito`, npm for both extensions, GitHub Actions; grouped, limit 3 open PRs per ecosystem)

Expect separate PRs for `chrome-extension` and `firefox-extension` when both are
affected — that is OK for now.

CI:

- [`go-test.yml`](../.github/workflows/go-test.yml) — `go test -race` (+ warn-only `govulncheck`)
- [`extension-build.yml`](../.github/workflows/extension-build.yml) — `npm ci` + `npm run build` for both extensions
- [`deps-audit.yml`](../.github/workflows/deps-audit.yml) — warn-only `govulncheck` + `npm audit --audit-level=high`
- Release tags also package extensions and update the Homebrew cask in [`infomosquitos/homebrew-tap`](https://github.com/infomosquitos/homebrew-tap) (see [`release.yml`](../.github/workflows/release.yml))

## Coming from Docker

Docker / Postgres packaging is removed. See [README.md](../README.md#coming-from-docker).
