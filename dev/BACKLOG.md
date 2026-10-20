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
