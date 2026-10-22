# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.2] - 2026-10-22

### Added
- Content search in the browse pages: space and catalog indexes served at `http://localhost:56403/browse/` search page text (same full-text search as MCP) and highlight matches. REST `/api/search` returns `browse_url` per result and supports `highlight=1`.

## [0.7.1] - 2026-10-22

### Fixed
- Confluence pages without content of their own (typically parents that only group other pages) are saved as empty pages; their `index.html` lists the child pages. Previously they fell back to the headless browser, which saved Jira's "Oops, you've found a dead link" page instead.
- "View in Confluence" links and saved `confluence_url` on Confluence Cloud include `/wiki` (they led to a 404).
- The browser fallback no longer saves Atlassian error pages as page content.
- Page tree keeps its full depth: parents are recorded during discovery, so pages saved via the browser fallback no longer appear at the top level of the space index.

Run `spacemosquito crawl --force <space URL>` once to repair pages saved by earlier versions.

## [0.7.0] - 2026-10-22

### Added
- Homebrew install on macOS: `brew install --cask infomosquitos/tap/spacemosquito`.
- Offline catalog navigation: `saved/index.html` lists spaces, each space gets an `index.html` page tree with a title filter (plus `index.md` versions), and pages link back to their space. Built after each crawl and by `reindex --content`.
- Extension **Spaces** tab: 📖 button opens a space's saved page tree in a new tab, served by `spacemosquito serve` at `http://localhost:56403/browse/`.

### Fixed
- Saved `index.html` for API-crawled pages now opens in a browser with images, diagrams, code blocks and tables (was raw Confluence markup). Run `spacemosquito reindex --content` to fix existing pages.
- Numbered lists that Confluence continues after other content (e.g. step 2 after a screenshot) keep their numbering in `content.md`.

### Changed
- HTTP request start/complete logs are **debug** (was info) to reduce spam.
- **Breaking:** `GET /api/session/status` requires `?url=`; `POST /api/session/validate` requires `confluence_url`. Unknown `session.enc` payloads are rejected (delete the session and re-capture).
- Browser extensions build with **TypeScript 7** + **esbuild** (`tsc --noEmit` then bundle); webpack/`ts-loader` removed.

## [0.6.0] - 2026-09-24

### Changed
- **Breaking**: Default HTTP/MCP listen port is **56403** (was 8081). Update extension backend URL and MCP client configs.

## [0.5.0] - 2026-09-17

### Changed
- **Breaking:** MCP transport is Streamable HTTP only (`POST /mcp` JSON). Deprecated HTTP+SSE (`GET /mcp` + `/mcp/session/<id>`) removed. Reconfigure agents with http / streamable-http (not SSE). See [Configure MCP](docs/guides/configure-mcp.md).

## [0.4.0] - 2026-09-16

### Added
- Toolbar icon animates (mosquito sucking blood) while any crawl is running.
- Spaces tab tooltip shows `Stored: N CSF · M HTML`.
- Extension Page tab shows stored body format (**CSF** / **HTML**).
- Extension popup remembers last tab (Page / Spaces / Settings).
- Extension toolbar icon: **green** / **gray**.
- Extension light/dark themes.
- New setting: auto-renew Confluence session.

### Changed
- Extension brand accents + active icon to muted green (`#4a7a5c`).
- Extension icon set.
- Extension popup major redesign.

### Fixed
- Spaces list could show crawled > total (e.g. 120/118) after a re-crawl.
- Session disc could stay green after Confluence auth died.
- Extension Page tab: Misdetection of the Confluence Cloud `homepageId`.
- Extension popup backend probe failure.

## [0.3.4] - 2026-09-04

### Fixed
- Several issues related to self-hosted wikis

### Changed
- On-disk page directories naming switched to `{confluence-id}-{title}`

### Added
- Support for capturing sessions from multiple confluence hosts.

## [0.3.3] - 2026-08-21

### Changed
- Firefox releases ship a signed `.xpi` for persistent install via Install Add-on From File (replacing temporary zip + `about:debugging` for end users).
- Browser extension display name is **Space Mosquito**.

## [0.3.2] - 2026-08-04

### Fixed
- Firefox extension Capture Session fixes.

## [0.3.1] - 2026-08-04

### Fixed
- Firefox extension Capture Session fixes.

## [0.3.0] - 2026-08-04

### Added
- `spacemosquito crawl-page <space-key> <confluence-id>` refreshes one page via the REST API, always overwriting local text and assets.

### Fixed
- Session Validate fixes.
- `GET /api/cron/config` panics fixes.

## [0.2.0] - 2026-08-03

### Added
- GitHub Releases and local `build-release.sh` publish Firefox/Chrome extension zips.

### Fixed
- Crawl Cancel now really cancels the started crawl.

### Changed
- Crawl skips already-downloaded non-empty assets. `crawl --force` re-scrapes every page and re-downloads every asset.

## [0.1.0] - 2026-08-02

### Added
- Rule-based **Confluence Storage Format → Markdown** converter (`internal/contentmd/csf`): macros (code, panel/info/note/warning/tip, jira, status, expand, toc, draw.io), task lists, native tables/links/emphasis, images and draw.io diagrams extracted as assets, emoticons mapped to Unicode. Replaces the generic converter on the API path; browser-fallback rendered HTML still uses the generic converter, selected by a format sniff.
- `metadata.json` records `body_format` (`storage`/`rendered`); `reindex --content` and `bootstrap import-saved` route each page to the correct converter (sniffing `raw.html` for legacy pages).
- `metadata.json` `diagrams` field for extracted draw.io previews.
- Chrome extension port (Manifest V3).
- REST API-first crawling with headless fallback.
- Multi-flavor support (Cloud / Server / Data Center).
- Standardized MCP SSE transport.
- Variable expansion in `config.yaml`.
- `AGENTS.md` for local development.

### Changed
- Removed the 50 KB `pages.content`/`content.md` truncation cap; full Markdown is stored (disk + DB + FTS).
- Unified all HTTP services on port 8081.
- Refactored session deletion for Docker compatibility.
- Broadened URL detection for custom domains.

### Fixed
- Asset downloads (images, diagrams, attachments) now send the session cookies, so SSO-protected instances return the file instead of a login page. Responses that are HTML (a login/redirect page) are rejected as a loud error rather than saved as a corrupt image.
- "Page Not Found" errors by using direct API extraction.
- "Device or resource busy" mount errors.
- Standardized MCP JSON-RPC handshake.
