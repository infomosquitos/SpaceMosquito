# Backlog

Flat task list. Status values: `backlog` | `ready` | `in_progress` | `blocked` | `done` | `cancelled`.

On cleanup, move shipped work to [`CHANGELOG.md`](../CHANGELOG.md) and clean the backlog — see [`CLEANUP.md`](CLEANUP.md). On version cut, see [`RELEASE.md`](RELEASE.md).

Reorder freely. Cancel by setting status to `cancelled`. Unexpected work becomes a new task with its own ID.

## Format

Each task:

```markdown
## <task-id>
- status: backlog
- parent: <optional-related-task-id>
- blocked_by: <optional-task-id>
- goal: One sentence
- done_when: Observable acceptance criteria
- notes: Optional one-liner
```

Detail lives in `dev/tasks/<task-id>.md` when needed (no `task-` filename prefix).

---

## extension-browse-space-link
- status: done
- parent: catalog-navigation-index
- goal: Open a space's saved page tree from the extension Spaces tab.
- done_when: `serve` serves the catalog at `/browse/` (no listings, no traversal); `/api/spaces` returns `browse_path`; both extensions show a 📖 button that opens it; tests green; docs updated.

## catalog-navigation-index
- status: done
- goal: Browse the saved catalog offline: root list of spaces and per-space page tree (HTML + Markdown), pages link back to the space index.
- done_when: `crawl`, `crawl-page`, `reindex --content` write `saved/index.{html,md}` and `saved/<SPACE>/index.{html,md}`; links resolve for names with spaces/special chars; tests green; docs updated.

## index-html-from-markdown
- status: done
- goal: Make saved `index.html` of API-crawled (Storage Format) pages browsable offline with images by rendering it from `content.md`.
- done_when: Crawl, `reindex --content` and import write Markdown-rendered `index.html` for CSF pages; local image links resolve; tests green; ADR-015/README/CHANGELOG updated.

## upgrade-extensions-typescript-7
- status: done
- goal: Make chrome/firefox extension builds work on TypeScript 7 (same as ticketmosquito: esbuild + tsc --noEmit; drop webpack/ts-loader).
- done_when: Both extensions `npm ci && npm run build` on typescript@7; Extension Build CI green; no ~5.9 pin; CHANGELOG/docs.
- notes: Done — esbuild + typescript@7. [`tasks/upgrade-extensions-typescript-7.md`](tasks/upgrade-extensions-typescript-7.md).

## deps-vuln-automation
- status: done
- goal: Automate vuln detection/updates for npm extensions + Go (Dependabot security updates; CI warn-only audit/govulncheck).
- done_when: Alerts+security updates on; govulncheck + npm audit warn in CI; dependabot.yml; docs note.
- notes: Done — CI + monthly `dependabot.yml` (from ticketmosquito). Enable alerts/security updates in GitHub Settings if needed. [`tasks/deps-vuln-automation.md`](tasks/deps-vuln-automation.md).

## cme-markdown-gap-analysis
- status: ready
- goal: Extract confluence-markdown-exporter conversion behaviors (tests + Converter) and gap-analyze vs our CSF→Markdown rules; recommend follow-ups only.
- done_when: CME capability inventory, ours crosswalk, severity-rated gap matrix, and ≤10 ranked implement/defer/ignore recommendations; no port in this task.
- notes: Detail in [`tasks/cme-markdown-gap-analysis.md`](tasks/cme-markdown-gap-analysis.md).
