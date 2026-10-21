# Dependency vulnerability scanning & auto-updates (npm + Go)

- **Task ID:** `deps-vuln-automation`
- **Status:** done
- **Parent:** —
- **Blocked by:** —

## Goal

Dependabot security updates + monthly version bumps + warn-only CI for npm and Go.

## Decisions (locked)

| # | Decision |
|---|----------|
| 1 | Dependabot |
| 2 | Alerts + security updates; **monthly** version bumps (copied from ticketmosquito; was “no schedule”, then superseded) |
| 3 | CI warn-only |
| 4 | Security PRs + monthly grouped version PRs |
| 5 | Dual Chrome/Firefox PRs OK |
| 6 | `govulncheck` |
| 7 | Actions included |

## Shipped

- [`.github/dependabot.yml`](../../.github/dependabot.yml) — monthly grouped updates (gomod `/spacemosquito`, both npm trees, github-actions)
- [`.github/workflows/deps-audit.yml`](../../.github/workflows/deps-audit.yml) — `govulncheck` + per-extension `npm audit --audit-level=high` (`continue-on-error`)
- `govulncheck` on [`go-test.yml`](../../.github/workflows/go-test.yml); `npm audit` before extension build on release
- Maintainer note in [`dev/README.md`](../README.md)

## Manual step (repo settings)

Enable **Dependabot alerts** + **Dependabot security updates** in GitHub
(Settings → Code security and analysis) if not already on after the org transfer.

## Done when

- [x] CI warn-only scans for Go + npm
- [x] Maintainer note
- [x] `dependabot.yml` monthly version updates
- [x] Dependabot alerts + security updates toggled on in GitHub UI (operator)
