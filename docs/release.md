# macOS release

v0.1.0-alpha.1 is a private GitHub pre-release built from `main` `546a2e1`. It is macOS only: Apple Silicon, Intel, and a universal binary. The packages are ad-hoc signed and not notarized.

Linux and Windows builds are not in this tag. Themes (PR #4) and the plugin creator (PR #5) are not in it either.

The repo is private. Downloads work with your GitHub login. `install.sh` and the commands below call `gh release download`, which uses the login from `gh auth login`. The script does not embed a token and does not prompt for one.

## One-time login

```bash
gh auth login
```

## Recommended install

```bash
gh release download v0.1.0-alpha.1 --repo vicgupta/dap --pattern install.sh && sh install.sh
```

The script picks arm64 or amd64 from `uname -m`, checks SHA256 against `checksums.txt`, and installs:

| Piece | Location |
| --- | --- |
| `DAP.app` | `/Applications` when that folder is writable, otherwise `~/Applications` |
| `da` | `~/.local/bin/da` |

Add `~/.local/bin` to `PATH` if it is not already there. Double-click `DAP.app` to run `da run`. Data stays in `~/.dap`, or in `$DAP_HOME` when that is set. The script clears `com.apple.quarantine` on the app and the CLI.

## Direct download

Release page: <https://github.com/vicgupta/dap/releases/tag/v0.1.0-alpha.1>

You have to be logged into GitHub. The same files are available with `gh`:

```bash
gh release download v0.1.0-alpha.1 --repo vicgupta/dap --pattern DAP-0.1.0-alpha.1-macos-arm64.zip --pattern checksums.txt
grep DAP-0.1.0-alpha.1-macos-arm64.zip checksums.txt | shasum -a 256 -c -
```

| Asset | What it is |
| --- | --- |
| `DAP-0.1.0-alpha.1-macos-arm64.zip` | Apple Silicon `DAP.app` |
| `DAP-0.1.0-alpha.1-macos-amd64.zip` | Intel `DAP.app` |
| `DAP-0.1.0-alpha.1-macos-universal.zip` | `DAP.app` with a fat binary |
| `da-0.1.0-alpha.1-darwin-arm64.tar.gz` | Apple Silicon `da` |
| `da-0.1.0-alpha.1-darwin-amd64.tar.gz` | Intel `da` |
| `da-0.1.0-alpha.1-darwin-universal.tar.gz` | Fat `da` |
| `checksums.txt` | SHA256 of the zips, tarballs, and `install.sh` |
| `install.sh` | Installer above |

Unzip and move `DAP.app` where you want it. The CLI inside the app is `DAP.app/Contents/MacOS/da`. A CLI tarball contains only the `da` binary. `tar -xzf da-0.1.0-alpha.1-darwin-arm64.tar.gz` writes `./da`.

Each app zip also contains `INSTALL.txt`, `LICENSE`, an icon, and the example plugins at `DAP.app/Contents/Resources/plugins`.

## Gatekeeper

Ad-hoc signing is the same idea as `codesign -s -`. There is no Apple Developer ID and no notarization, so the first open after a browser download is blocked.

Pick one:

1. Right-click `DAP.app` and choose **Open**, then **Open** again.
2. **System Settings > Privacy & Security > Open Anyway**.
3. In Terminal, from the folder that contains the app:

```bash
xattr -dr com.apple.quarantine DAP.app
```

`install.sh` already runs that `xattr` command.

## After install

Example plugins need `python3` on `PATH`. From the app you installed under `/Applications`:

```bash
da plugin install /Applications/DAP.app/Contents/Resources/plugins/mail-fake --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/inbox-followups --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/hello-watch --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/workflow --approve
da run
```

Use `~/Applications/...` if the app landed there. Gmail and Outlook need an OAuth client id on the Connections page. The steps are in [inbox-followups.md](inbox-followups.md). The demo mailbox (`mail-fake`) does not. Nothing sends until you approve a card.

`mail-gmail` and `mail-graph` are in the app bundle so those client ids have a plugin to install. They do not include a client id.

## Version string

The v0.1.0-alpha.1 binaries were built from `main` before `da version` existed. `da --version` on that build exits 2. `da doctor` prints `"version": "0.1.0"`. The app plist short version is `0.1.0-alpha.1`.

Source after this workflow can stamp a later tag:

```text
go build -ldflags "-X github.com/vicgupta/dap/internal/version.Release=0.1.0-alpha.2" -o da ./cmd/da
da version
```

`version` stays `0.1.0` so plugin `min_dap_version` checks keep working. `release` is the tag without the leading `v`.

## Known issues

The chromeless window needs Google Chrome or Microsoft Edge. If neither is installed, `da run` opens the URL with `open` (the default browser).

Two `da` processes cannot use the same data directory at once.

## Cutting the next tag

Pushing a `v*` tag runs `.github/workflows/release.yml` once that workflow is on the default branch. The tag `v0.1.0-alpha.1` points at `546a2e1`, which does not contain the workflow, so Actions did not build that release. It was packaged on a Linux machine and uploaded with `gh`.

The workflow cross-compiles on `ubuntu-latest`, builds a universal binary with `lipo`, ad-hoc signs with `rcodesign` (the signature flag is `ADHOC`, same outcome as `codesign -s -`), and publishes a GitHub release. Tags that contain `-alpha`, `-beta`, or `-rc` are marked pre-release.

Local equivalent, without uploading:

```bash
packaging/macos/release.sh v0.1.0-alpha.2
```

Artifacts land in `dist/release/`. On a Mac the script uses `lipo` and `codesign -s -` when those tools are on `PATH`. On Linux it uses `llvm-lipo` and `rcodesign`.

To publish from a checkout that already has the artifacts:

```bash
packaging/macos/publish.sh v0.1.0-alpha.2
```

That command uses the current `gh` login.
