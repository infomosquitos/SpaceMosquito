# Release

Related: [CHANGELOG.md](../CHANGELOG.md), [CLEANUP.md](CLEANUP.md)

Promote finished task artifacts with [CLEANUP.md](CLEANUP.md) first if needed (backlog rows, task detail files, STATUS). This runbook is for **versioning and publishing**.

## Prepare

1. Ensure the build is green: `make test`
2. Update [`CHANGELOG.md`](../CHANGELOG.md) to the new version (move items out of Unreleased). Keep changelog entries concise, focus on user-facing functionality changes, skip technical details.
3. Confirm cleanup is done (or run [`CLEANUP.md`](CLEANUP.md) now).
4. Bump firefox-extension and chrome-extension `manifest.json` + `package.json` to X.Y.Z
   (release CI also stamps the version into the packaged manifests).
5. Ensure GitHub repository secrets `AMO_JWT_ISSUER` and `AMO_JWT_SECRET` are set
   (AMO API credentials for signing the Firefox XPI). Without them the
   `build-extensions` job fails.
6. Ensure GitHub repository secret `HOMEBREW_TAP_TOKEN` is set and not expired:
   a fine-grained PAT (resource owner `infomosquitos`, repository
   [`homebrew-tap`](https://github.com/infomosquitos/homebrew-tap) only,
   **Contents: Read and write**). Without it the `homebrew` job fails after the
   release is published; rotate it before it expires.

## Cut the release

```sh
git tag vX.Y.Z         # use the version from CHANGELOG
git push origin vX.Y.Z
```

Watch **Actions → Release**. It tests, builds four binaries + signed Firefox
XPI + Chrome extension zip, uploads them + `SHA256SUMS`, and creates the GitHub
Release for that tag. Then the `homebrew` job renders `Casks/spacemosquito.rb`
with [`scripts/render-homebrew-cask.sh`](../scripts/render-homebrew-cask.sh)
and pushes it to the tap (skipped for pre-release tags containing `-`).

Alternative: GitHub **Releases → Draft a new release → create tag `vX.Y.Z` → Publish**. That also pushes the tag and triggers the same workflow (assets attach to the release).

## After

1. Confirm assets on the release page:
   - `spacemosquito-{darwin,linux,windows}-*` binaries
   - `spacemosquito-firefox-v*.xpi` and `spacemosquito-chrome-v*.zip`
   - `SHA256SUMS` covering all of the above
2. Spot-check: Firefox — install XPI via `about:addons` → Install Add-on From File.
   Chrome — unzip so `manifest.json` is at the archive root, then Load unpacked.
3. Confirm the tap commit `spacemosquito X.Y.Z` landed in
   [`infomosquitos/homebrew-tap`](https://github.com/infomosquitos/homebrew-tap/commits),
   then spot-check: `brew update && brew upgrade --cask spacemosquito && spacemosquito version`.
4. Set [`STATUS.md`](STATUS.md) next focus (usually next backlog item).

## Local release artifacts (no GitHub)

Produces the same binary + extension layout as CI when AMO env vars are set:

```sh
cd spacemosquito
export AMO_JWT_ISSUER=… AMO_JWT_SECRET=…   # optional; without them Firefox XPI is skipped
./scripts/build-release.sh vX.Y.Z
ls dist/
```
