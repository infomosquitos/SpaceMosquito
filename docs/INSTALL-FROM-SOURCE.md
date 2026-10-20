# Install from source

Build the `spacemosquito` binary and browser extensions from a git clone.

For pre-built binaries and extension packages, see [Install](INSTALL.md).

## Requirements

- Go 1.25+
- npm (and npx) for building extensions
- Firefox or Chrome (for the Space Mosquito extension)

Optional (only to **regenerate** toolbar PNG icons from SVG):

- [librsvg](https://gitlab.gnome.org/GNOME/librsvg) (`rsvg-convert`): `brew install librsvg`  

Committed `*-extension/assets/icon-{active,inactive}-*.png` are enough for a normal
`make build-extensions`; the render step is skipped when those tools are missing.

## Binary

```sh
git clone https://github.com/infomosquitos/SpaceMosquito.git
cd SpaceMosquito
make build
# binary: spacemosquito/spacemosquito
```

Cross-platform release artifacts (binaries + extensions, same layout as CI):

```sh
cd spacemosquito
# Firefox XPI signing needs AMO_JWT_ISSUER + AMO_JWT_SECRET in the environment;
# without them the script skips the Firefox XPI and still builds Chrome.
./scripts/build-release.sh v0.2.0
ls dist/
```

Extension packages only:

```sh
./scripts/build-extension-zips.sh v0.2.0
# With signing:
#   AMO_JWT_ISSUER=… AMO_JWT_SECRET=… ./scripts/build-extension-zips.sh v0.2.0
```

## Browser extensions

```sh
make build-extensions
# or:
#   cd firefox-extension && npm ci && npm run build
#   cd chrome-extension && npm ci && npm run build
```

### Firefox (development)

Open `about:debugging` → **This Firefox** → **Load Temporary Add-on…** → select
`firefox-extension/dist/manifest.json`. Temporary add-ons are removed when
Firefox restarts.

For a persistent install of a **release** build, use the signed `.xpi` from
GitHub Releases ([Install](INSTALL.md#firefox)).

### Chrome

Open `chrome://extensions` → **Developer mode** → **Load unpacked** → select
`chrome-extension/dist/`.

## Next steps

```sh
./spacemosquito/spacemosquito init
./spacemosquito/spacemosquito serve
```

Then capture a Confluence session with the extension (see [README](../README.md#capture-a-confluence-session)).

Contributor workflows, tests, and tooling: [dev/README.md](../dev/README.md).
