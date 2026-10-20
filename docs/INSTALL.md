# Install

Install SpaceMosquito from a [GitHub Release](https://github.com/infomosquitos/SpaceMosquito/releases).

## Requirements

- A release asset for your OS/arch (`spacemosquito-*`)
- Firefox or Chrome (for the Space Mosquito extension)

## Binary

1. Download the binary for your platform and optionally `SHA256SUMS` from the release.
2. Verify the checksum against `SHA256SUMS`.
3. Install onto your `PATH` (example for macOS/Linux):

```sh
chmod +x spacemosquito-darwin-arm64
sudo mv spacemosquito-darwin-arm64 /usr/local/bin/spacemosquito
```

## Browser extension

Each release also includes:

- `spacemosquito-firefox-*.xpi` — signed Firefox add-on
- `spacemosquito-chrome-*.zip` — Chrome unpacked package

### Firefox

1. Download `spacemosquito-firefox-*.xpi` from the release.
2. Open `about:addons` → gear icon → **Install Add-on From File…**
3. Select the `.xpi` file and confirm.

The add-on stays installed across Firefox restarts.

### Chrome

1. Unzip `spacemosquito-chrome-*.zip` into a folder so `manifest.json` is at the
   top of that folder (do not load a parent that only contains another folder).
2. Open `chrome://extensions` → enable **Developer mode** → **Load unpacked**
3. Select that folder (the one that contains `manifest.json`).

## Next steps

```sh
spacemosquito init
spacemosquito serve
```

Then capture a Confluence session with the extension (see [README](../README.md#capture-a-confluence-session)).
