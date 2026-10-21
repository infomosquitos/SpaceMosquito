# Upgrade Chrome/Firefox extensions to TypeScript 7

- **Task ID:** `upgrade-extensions-typescript-7`
- **Status:** done
- **Parent / blocked by:** —

## Goal

Both `chrome-extension` and `firefox-extension` build cleanly on **TypeScript 7**
(same toolchain for both), with Dependabot able to track TS7+ without breaking
the extension pipeline.

## Decisions (locked)

| # | Topic | Decision |
|---|--------|----------|
| 1 | Build approach | Drop webpack/`ts-loader`. **esbuild** bundles; **TypeScript 7** (`tsc --noEmit`) typechecks only. |
| 2 | Typecheck vs emit | `npm run build` = `tsc --noEmit` then esbuild. |
| 3 | Shared config | Duplicated per extension (no shared build package). |
| 4 | Dependabot guard | Allow typescript majors; gate is `Extension Build` CI. |
| 5 | Minimum Node | `engines.node` `>=16.20.0`; CI Node 22. |

## Shipped

- `typescript@^7`, `esbuild`; removed webpack/`ts-loader`/`copy-webpack-plugin`
- `esbuild.mjs` per extension (`background` + `popup`; keeps `webextension-polyfill`)
- Root `Makefile` / `scripts/build-extension-zips.sh` call `npm run build`
- CHANGELOG Unreleased note

## Done when

- [x] Both extensions `npm ci && npm run build` on typescript@7
- [x] Typecheck in build script
- [x] No `~5.9` pin; webpack stack gone
- [x] Docs/CHANGELOG updated

## Notes

```text
npm run build  →  tsc --noEmit  &&  node esbuild.mjs
```

Reference: TicketMosquito `upgrade-extensions-typescript-7`.
