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

## cme-markdown-gap-analysis
- status: ready
- goal: Extract confluence-markdown-exporter conversion behaviors (tests + Converter) and gap-analyze vs our CSF→Markdown rules; recommend follow-ups only.
- done_when: CME capability inventory, ours crosswalk, severity-rated gap matrix, and ≤10 ranked implement/defer/ignore recommendations; no port in this task.
- notes: Detail in [`tasks/cme-markdown-gap-analysis.md`](tasks/cme-markdown-gap-analysis.md).

## session-enc-legacy-cleanup
- status: backlog
- parent: multi-wiki-sessions
- goal: After a release window, remove legacy single-session session.enc migration and Save/Load shims.
- done_when: v2-only blob; no bare Session file unmarshal; API requires host/URL; tests + CHANGELOG/ADR.
- notes: Detail in [`tasks/session-enc-legacy-cleanup.md`](tasks/session-enc-legacy-cleanup.md). Defer until multi-session builds have been in the wild. Parent task shipped in 0.3.4 (ADR-002).
