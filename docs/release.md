# macOS test releases

DAP ships unsigned macOS builds. There is no Apple Developer account and no notarization. A tag named `v*` builds Apple Silicon, Intel, and a universal binary, then publishes a GitHub release. Tags that contain a hyphen (`v0.1.0-alpha.1`) are marked pre-release.

The first cut, [v0.1.0-alpha.1](https://github.com/vicgupta/dap/releases/tag/v0.1.0-alpha.1), was packaged from `main` at `546a2e1` before `da version` existed. That binary reports `0.1.0` from `da doctor`. Builds from this workflow pass `-ldflags` so `da version` prints the tag while plugin compatibility stays on core `0.1.0`.

## What a release contains

- `DAP-<version>-macos-arm64.zip`, `amd64`, and `universal`: a `DAP.app` bundle. Double-click runs `da run`.
- `da-<version>-darwin-arm64.tar.gz`, `amd64`, and `universal`: the `da` CLI alone.
- `checksums.txt`: SHA256.

`DAP.app` layout:

```text
DAP.app/Contents/Info.plist
DAP.app/Contents/MacOS/DAP     launcher
DAP.app/Contents/MacOS/da      CLI
DAP.app/Contents/Resources/AppIcon.icns
DAP.app/Contents/Resources/plugins/
```

Data stays in `~/.dap`, or `$DAP_HOME` if set. The app does not move data to `~/Library/Application Support`.

## Install

Download one zip from the GitHub release. Apple Silicon is `macos-arm64`. Intel is `macos-amd64`. `macos-universal` runs on both.

```bash
shasum -a 256 -c checksums.txt
unzip DAP-0.1.0-alpha.1-macos-arm64.zip
```

Move `DAP.app` to `/Applications` or `~/Applications`.

### Gatekeeper

The app is ad-hoc signed (`codesign -s -`) and not notarized. macOS blocks the first open after a browser download.

- Right-click `DAP.app` and choose **Open**, then **Open** again.
- Or **System Settings > Privacy & Security > Open Anyway**.
- Or, in Terminal, from the folder that contains the app:

```bash
xattr -dr com.apple.quarantine DAP.app
```

## Plugins

Example plugins are inside the app and need Python 3 (`python3` on `PATH`):

```bash
da plugin install /Applications/DAP.app/Contents/Resources/plugins/hello-watch --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/workflow --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/mail-fake --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/inbox-followups --approve
da run
```

Gmail and Outlook need an OAuth client id on the Connections page. See [inbox-followups.md](inbox-followups.md). The demo mailbox does not. Nothing sends until you approve a card.

## Known issues

- The chromeless window needs Google Chrome or Microsoft Edge. Otherwise `da run` opens the URL in the default browser.
- Gatekeeper warns on the first launch. Steps are above.
- Two copies of `da` cannot use the same data directory at once.
- Themes and the in-app plugin creator are not in `v0.1.0-alpha.1`.

## Cutting the next tag

```bash
git checkout main
git pull
git tag v0.1.0-alpha.2
git push origin v0.1.0-alpha.2
```

GitHub Actions runs `scripts/package-macos.sh` on `macos-latest` and uploads the zip files, tarballs, and checksums. Do not force-push a tag that already has a release.
