# Dependency vulnerability scanning & auto-updates (npm + Go)

- **Task ID:** `deps-vuln-automation`
- **Status:** done
- **Parent:** —
- **Blocked by:** —

## Goal

Dependabot security updates + warn-only CI for npm and Go. No weekly version bumps.

## Decisions (locked)

| # | Decision |
|---|----------|
| 1 | Dependabot |
| 2 | Alerts + security updates; no scheduled version bumps |
| 3 | CI warn-only |
| 4 | Security-only PRs |
| 5 | Dual Chrome/Firefox PRs OK |
| 6 | `govulncheck` |
| 7 | Actions included (via Dependabot security updates) |

## Shipped

- [`.github/workflows/deps-audit.yml`](../../.github/workflows/deps-audit.yml) — `govulncheck` + per-extension `npm audit --audit-level=high` (`continue-on-error`)
- `govulncheck` on [`go-test.yml`](../../.github/workflows/go-test.yml); `npm audit` before extension build on release
- Maintainer note in [`dev/README.md`](../README.md)

## Manual step (repo settings)

Enable **Dependabot alerts** + **Dependabot security updates** in GitHub
(Settings → Code security and analysis). Cannot be fully expressed as a committed
file without scheduled version updates.

## Done when

- [x] CI warn-only scans for Go + npm
- [x] Maintainer note
- [x] No weekly version-update schedule
- [ ] Dependabot alerts + security updates toggled on in GitHub UI (operator)
