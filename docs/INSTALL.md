# Install

Install SpaceMosquito from a [GitHub Release](https://github.com/infomosquitos/SpaceMosquito/releases).

## Requirements

- A release asset for your OS/arch (`spacemosquito-*`)
- Firefox or Chrome (for the Space Mosquito extension)

## Homebrew (macOS)

```sh
brew install --cask infomosquitos/tap/spacemosquito
```

Upgrade with `brew upgrade --cask spacemosquito`. Uninstall with
`brew uninstall --cask spacemosquito`; add `--zap` to also delete
`~/.spacemosquito` (catalog, config and encryption key).

The cask installs the CLI binary only — install the
[browser extension](#browser-extension) separately. Linux and Windows: use the
binary below.

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

Chrome loads the extension from a **folder**, not from the `.zip` (selecting the
zip fails with *Manifest file is missing or unreadable*).

1. Unzip `spacemosquito-chrome-*.zip` into a folder you keep — Chrome reads it
   on every start, so the extension breaks if the folder is deleted:

   ```sh
   mkdir -p ~/Applications/SpaceMosquito-Chrome
   unzip -o ~/Downloads/spacemosquito-chrome-v*.zip -d ~/Applications/SpaceMosquito-Chrome
   ```

2. Open `chrome://extensions` → enable **Developer mode** → **Load unpacked**
3. Select that folder (the one that contains `manifest.json`). In the macOS file
   picker, **Cmd+Shift+G** lets you type the path.

To upgrade, unzip the new release into the same folder (`-o` overwrites), then
click the reload icon on the extension card in `chrome://extensions`.

## Next steps

```sh
spacemosquito init
spacemosquito serve
```

Then capture a Confluence session with the extension (see [README](../README.md#capture-a-confluence-session)).
