#!/usr/bin/env bash
# Build Chrome extension zip and (when AMO credentials are set) a signed
# Firefox XPI for GitHub Releases.
# Usage: build-extension-zips.sh <version> [out_dir]
#   version: tag or semver (e.g. v0.1.0 or 0.1.0). Artifact names keep the given
#   string; manifest.json gets the leading "v" stripped.
#
# Firefox signing requires env:
#   AMO_JWT_ISSUER, AMO_JWT_SECRET
# Set REQUIRE_FIREFOX_XPI=1 to fail if signing is skipped (used by CI).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:?usage: build-extension-zips.sh <version> [out_dir]}"
OUT_DIR="${2:-$REPO_ROOT/dist}"
MANIFEST_VERSION="${VERSION#v}"

mkdir -p "$OUT_DIR"
# Absolute path: zip runs from <ext>/dist, so a relative OUT_DIR like "dist"
# would resolve to <ext>/dist/dist/... and fail.
OUT_DIR="$(cd "$OUT_DIR" && pwd)"

# Manifest backup paths must be global: EXIT traps run after function locals
# are torn down (set -e), so locals would be unbound under `set -u`.
_SM_MANIFEST_PATH=""
_SM_MANIFEST_BAK=""

restore_manifest() {
  if [[ -n "${_SM_MANIFEST_BAK}" && -n "${_SM_MANIFEST_PATH}" && -f "${_SM_MANIFEST_BAK}" ]]; then
    mv "${_SM_MANIFEST_BAK}" "${_SM_MANIFEST_PATH}" 2>/dev/null || true
  fi
  _SM_MANIFEST_PATH=""
  _SM_MANIFEST_BAK=""
}

begin_stamped_manifest() {
  local dir="$1"
  _SM_MANIFEST_PATH="$dir/manifest.json"
  _SM_MANIFEST_BAK="${_SM_MANIFEST_PATH}.bak.$$"
  cp "${_SM_MANIFEST_PATH}" "${_SM_MANIFEST_BAK}"
  trap restore_manifest EXIT
  stamp_manifest "${_SM_MANIFEST_PATH}" "$MANIFEST_VERSION"
}

end_stamped_manifest() {
  trap - EXIT
  restore_manifest
}

stamp_manifest() {
  local path="$1"
  local ver="$2"
  node -e '
    const fs = require("fs");
    const path = process.argv[1];
    const ver = process.argv[2];
    const m = JSON.parse(fs.readFileSync(path, "utf8"));
    m.version = ver;
    fs.writeFileSync(path, JSON.stringify(m, null, 2) + "\n");
  ' "$path" "$ver"
}

build_chrome() {
  local dir="$REPO_ROOT/chrome-extension"
  local zip_path="$OUT_DIR/spacemosquito-chrome-${VERSION}.zip"

  echo "Building chrome-extension (manifest version $MANIFEST_VERSION)"
  begin_stamped_manifest "$dir"
  (
    cd "$dir"
    npm ci
    npm run build
  )
  rm -f "$zip_path"
  ( cd "$dir/dist" && zip -r "$zip_path" . )
  end_stamped_manifest
  echo "Wrote $zip_path"
}

build_firefox_xpi() {
  local dir="$REPO_ROOT/firefox-extension"
  local xpi_path="$OUT_DIR/spacemosquito-firefox-${VERSION}.xpi"
  local artifacts_dir
  local source_zip
  local source_base
  local signed

  echo "Building firefox-extension (manifest version $MANIFEST_VERSION)"
  begin_stamped_manifest "$dir"
  (
    cd "$dir"
    npm ci
    npm run build
  )

  if [[ -z "${AMO_JWT_ISSUER:-}" || -z "${AMO_JWT_SECRET:-}" ]]; then
    end_stamped_manifest
    if [[ "${REQUIRE_FIREFOX_XPI:-}" == "1" ]]; then
      echo "error: AMO_JWT_ISSUER and AMO_JWT_SECRET are required to sign the Firefox XPI" >&2
      exit 1
    fi
    echo "warning: skipping Firefox XPI (AMO_JWT_ISSUER / AMO_JWT_SECRET not set)" >&2
    return 0
  fi

  artifacts_dir="$(mktemp -d "${TMPDIR:-/tmp}/sm-ff-artifacts.XXXXXX")"
  # mktemp creates an empty file; zip refuses to treat that as a new archive.
  source_base="$(mktemp "${TMPDIR:-/tmp}/sm-ff-source.XXXXXX")"
  rm -f "$source_base"
  source_zip="${source_base}.zip"
  # Human-readable sources for AMO (production bundle is minified).
  (
    cd "$dir"
    zip -r "$source_zip" . \
      -x "node_modules/*" \
      -x "dist/*" \
      -x "web-ext-artifacts/*" \
      -x "*.bak.*" \
      -x ".git/*"
  )

  echo "Signing Firefox extension via AMO (unlisted)…"
  (
    cd "$dir"
    npx --yes web-ext@8 sign \
      --source-dir ./dist \
      --channel unlisted \
      --api-key "$AMO_JWT_ISSUER" \
      --api-secret "$AMO_JWT_SECRET" \
      --artifacts-dir "$artifacts_dir" \
      --upload-source-code "$source_zip"
  )
  rm -f "$source_zip"

  signed="$(find "$artifacts_dir" -maxdepth 1 -type f -name '*.xpi' | head -n 1 || true)"
  if [[ -z "$signed" ]]; then
    echo "error: web-ext sign produced no .xpi in $artifacts_dir" >&2
    ls -la "$artifacts_dir" >&2 || true
    end_stamped_manifest
    rm -rf "$artifacts_dir"
    exit 1
  fi

  rm -f "$xpi_path"
  mv "$signed" "$xpi_path"
  rm -rf "$artifacts_dir"

  end_stamped_manifest
  echo "Wrote $xpi_path"
}

build_firefox_xpi
build_chrome
