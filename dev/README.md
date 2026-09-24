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

## Coming from Docker

Docker / Postgres packaging is removed. See [README.md](../README.md#coming-from-docker).
