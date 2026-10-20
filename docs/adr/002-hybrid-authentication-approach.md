# ADR-002: Hybrid Authentication Approach

- **Status:** Accepted
- **Date:** 2025-01-17
- **Updated:** 2026-09-04 — multi-wiki sessions in one encrypted blob
- **Updated:** 2026-10-20 — status/validate require a host URL; unknown on-disk format fails closed

## Context

Users need to authenticate with Confluence (OAuth/SSO/2FA) on their machine. We need a reliable way to capture and reuse session credentials without exposing them. Operators may use **more than one Confluence host** (Cloud + self-hosted, multiple companies); capturing one site must not erase another.

## Decision

Use a hybrid authentication approach where the browser extension captures session cookies during interactive login and exports them to the Go backend via an **encrypted session file**.

- Direct form-based login from Go is fragile with modern Confluence (OAuth redirects, SSO, 2FA)
- The extension runs in a real browser where the user can complete all authentication flows naturally
- Session cookies can be captured after login and exported to the backend for automated crawl/cron runs
- Cookies are stored encrypted on disk (AES-GCM), never in plaintext
- Extension on the host talks to the local backend at localhost; cookies are exported into the encrypted session file under the data directory

**Multi-site storage:** `session.enc` decrypts to a JSON blob with `version` (currently `2`) and a `sessions` map of **hostname → session**. Capture **upserts** one hostname only. Crawl, validate, and cron **select** the session whose hostname matches the space/page URL. Status (`?url=`) and validate (`confluence_url`) require a host URL. Unknown or corrupt on-disk payloads fail closed — delete the session and re-capture. Extension list/delete UI for multiple sites is deferred to the extension redesign epic.

## Alternatives considered

| Option | Why not |
|--------|---------|
| Remote desktop / noVNC login | Clunky and harder for 2FA/SSO |
| OAuth client credentials | Users won't provide API tokens and we must "pretend to be a normal user" |
| Puppeteer/Playwright headless login | Fragile with modern auth flows, loses the real browser's session capabilities |
| One session file per host | More files to manage; single encrypted blob is enough for solo use |
| Key by full BaseURL (incl. context path) | Hostname-only is sufficient for v1 |

## Consequences

- Session file must be protected; encryption key should be user-provided or derived from OS keyring
- Session cookies expire; the extension must handle re-authentication gracefully
- The cookie exchange happens over localhost (encrypted or not), which is a trust boundary
- Missing session for a host fails clearly; operators capture per wiki they use
- Unknown on-disk format fails closed — delete session and re-capture
- Delete session still clears the **entire** blob (all hosts) until redesign adds per-host delete
