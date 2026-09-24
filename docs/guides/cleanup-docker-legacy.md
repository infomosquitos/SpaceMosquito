# Guide: Cleanup Legacy Docker State

Wipe leftover **Docker Compose** state from machines that ran SpaceMosquito with containers + PostgreSQL. Crawl data on the host (`saved-data/`, `session.enc`) is **not** deleted unless you opt in.

After cleanup, migrate via [README Coming from Docker](../../README.md#coming-from-docker).

## Prerequisites

- Docker CLI available (`docker` on `PATH`)
- Daemon running (Docker Desktop or Colima)

If Docker is missing or the daemon is down, the script prints host leftovers and migration steps, then exits successfully.

## Quick start

From the repo root:

```sh
chmod +x scripts/cleanup-docker-legacy.sh   # once

./scripts/cleanup-docker-legacy.sh --dry-run
./scripts/cleanup-docker-legacy.sh
```

## Options

| Flag | Effect |
|------|--------|
| `--dry-run` | Print actions only |
| `--project-name NAME` | Compose project name (default: repo directory basename). **Normalized to lowercase** (e.g. `SpaceMosquito` → `spacemosquito`) because Compose rejects mixed case. |
| `--purge-images` | Remove images whose names match `spacemosquito` / `{project}-app` |
| `--purge-bind-mounts` | **Destructive:** delete host `saved-data/`, `saved/`, `session.enc`, `config.yaml`, `cron-config.json`, `.env` under the repo |

## What it does

1. **Normalizes** the Compose project name to lowercase (`SpaceMosquito` → `spacemosquito`) so `docker compose -p` accepts it.
2. `docker compose down --remove-orphans` with the normalized project name.
3. Force-removes leftover containers (compose label or name match).
4. For each matching `*_pgdata` volume: remove containers still using it, then `docker volume rm`.
5. Optionally removes related images (`--purge-images`).
6. Lists bind-mount leftovers; deletes them only with `--purge-bind-mounts`.

## Manual equivalent

```sh
cd /path/to/SpaceMosquito
docker compose down --volumes --remove-orphans
docker volume ls | grep -i spacemosquito
# docker volume rm <name>   # if any remain
```

## Migrate to dockerless (after cleanup)

Postgres volume data is **not** imported. Rebuild the catalog from saved HTML:

```sh
cd spacemosquito
go build -o spacemosquito ./cmd/spacemosquito

./spacemosquito init
mkdir -p ~/.spacemosquito/saved
# If you still have the Compose bind mount:
cp -R ../saved-data/* ~/.spacemosquito/saved/   # adjust paths as needed

./spacemosquito bootstrap import-saved
./spacemosquito reindex --content
./spacemosquito serve
```

Point the Space Mosquito extension at `http://localhost:56403`.

## After packaging removal

Once `docker-compose.yml` is gone from the repo, the script still works via `--project-name` (normalized to lowercase):

```sh
./scripts/cleanup-docker-legacy.sh --project-name SpaceMosquito --dry-run
./scripts/cleanup-docker-legacy.sh
```
