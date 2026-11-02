# Releases

v0.1.0-alpha.1 is a private GitHub pre-release built from `main` `546a2e1`. That tag is macOS only: Apple Silicon, Intel, and a universal binary, ad-hoc signed and not notarized. It does not include Linux or Windows packages, and this repo does not replace that tag.

The release workflow on this branch, once it is on `main`, builds macOS, Linux, and Windows for a later `v*` tag. Themes (PR #4) and the plugin creator (PR #5) are not in v0.1.0-alpha.1.

The repo is private. Downloads use your GitHub login. `install.sh` and `install.ps1` call `gh release download`, which uses the login from `gh auth login`. The scripts do not embed a token and do not prompt for one.

## One-time login

```bash
gh auth login
```

## v0.1.0-alpha.1 (macOS)

Release page: <https://github.com/vicgupta/dap/releases/tag/v0.1.0-alpha.1>

```bash
gh release download v0.1.0-alpha.1 --repo vicgupta/dap --pattern install.sh && sh install.sh
```

The script picks arm64 or amd64 from `uname -m`, checks SHA256 against `checksums.txt`, and installs:

| Piece | Location |
| --- | --- |
| `DAP.app` | `/Applications` when that folder is writable, otherwise `~/Applications` |
| `da` | `~/.local/bin/da` |

Add `~/.local/bin` to `PATH` if it is not already there. Double-click `DAP.app` to run `da run`. Data stays in `~/.dap`, or in `$DAP_HOME` when that is set. The script clears `com.apple.quarantine` on the app and the CLI.

Direct download of one file:

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
| `checksums.txt` | SHA256 |
| `install.sh` | macOS installer for this tag |

Unzip and move `DAP.app` where you want it. The CLI inside the app is `DAP.app/Contents/MacOS/da`. A CLI tarball from this tag contains only the `da` binary. Each app zip also contains `INSTALL.txt`, `LICENSE`, an icon, and the example plugins at `DAP.app/Contents/Resources/plugins`.

### Gatekeeper

Ad-hoc signing is the same idea as `codesign -s -`. There is no Apple Developer ID and no notarization, so the first open after a browser download is blocked.

1. Right-click `DAP.app` and choose **Open**, then **Open** again.
2. **System Settings > Privacy & Security > Open Anyway**.
3. In Terminal, from the folder that contains the app:

```bash
xattr -dr com.apple.quarantine DAP.app
```

`install.sh` already runs that `xattr` command.

## Later tags: Linux and Windows

`packaging/release.sh` and `.github/workflows/release.yml` add these archives next to the macOS files. `checksums.txt` covers every file. Pre-release tags (`-alpha`, `-beta`, `-rc`) stay marked pre-release.

| Asset | What it is |
| --- | --- |
| `da-<version>-linux-amd64.tar.gz` | `da`, the `dap` launcher, and `plugins/` |
| `da-<version>-linux-arm64.tar.gz` | Same for arm64 |
| `dap_<debver>_amd64.deb` | `/usr/bin/da`, `/usr/bin/dap`, `/usr/share/dap/plugins` |
| `dap_<debver>_arm64.deb` | Same for arm64 |
| `DAP-<version>-windows-amd64.zip` | `da.exe`, `DAP.cmd`, and `plugins\` |
| `install.sh` | macOS and Linux |
| `install.ps1` | Windows |

`<version>` is the tag without `v`. `<debver>` is the same string with the first hyphen replaced by `~`, so `0.1.0-alpha.2` becomes `0.1.0~alpha.2`. There is no AppImage.

### Linux

```bash
gh release download <tag> --repo vicgupta/dap --pattern install.sh && sh install.sh
```

On Linux the script installs:

| Piece | Location |
| --- | --- |
| `da` and `dap` | `~/.local/bin` |
| plugins | `~/.local/share/dap/plugins` |

`dap` runs `da run`. Both commands have to stay in the same directory.

The `.deb` is the other install path, and it needs `sudo`:

```bash
gh release download <tag> --repo vicgupta/dap --pattern 'dap_*_amd64.deb' --pattern checksums.txt
grep amd64.deb checksums.txt | sha256sum -c -
sudo dpkg -i dap_*_amd64.deb
```

Use the arm64 deb on arm64. Plugins from the package are in `/usr/share/dap/plugins`.

The chromeless window needs Google Chrome, Chromium, or Microsoft Edge. Otherwise `dap` opens the URL with `xdg-open`.

### Windows

```powershell
gh release download <tag> --repo vicgupta/dap --pattern install.ps1
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The script installs into `%LOCALAPPDATA%\DAP`, prepends that folder to the user `PATH`, unblocks the files, and creates a Start menu shortcut named **DAP** that runs `da.exe run`. Double-click `DAP.cmd` in that folder to do the same. Data stays in `%USERPROFILE%\.dap`, or in `%DAP_HOME%`.

### SmartScreen

The Windows exe is unsigned. There is no Authenticode certificate. After a browser download, Windows may say **Windows protected your PC**.

Choose **More info**, then **Run anyway**.

In PowerShell, from the folder that contains the exe:

```powershell
Unblock-File .\da.exe
Unblock-File .\DAP.cmd
```

`install.ps1` unblocks the copy it installs. The chromeless window needs Microsoft Edge or Google Chrome. Otherwise DAP opens the default browser.

## After install

Example plugins need Python (`python3` on macOS and Linux; `python` or `py` on Windows).

```bash
da plugin install /Applications/DAP.app/Contents/Resources/plugins/mail-fake --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/inbox-followups --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/hello-watch --approve
da plugin install /Applications/DAP.app/Contents/Resources/plugins/workflow --approve
da run
```

Use the plugins directory the installer printed (`~/.local/share/dap/plugins`, `/usr/share/dap/plugins`, or `%LOCALAPPDATA%\DAP\plugins`). Gmail and Outlook connect through Composio: save the API key on Connections, then press Connect. The steps are in [composio.md](composio.md). The demo mailbox (`mail-fake`) does not need a key. Nothing sends until you approve a card. Direct Microsoft Graph, and legacy direct Gmail, are documented in [inbox-followups.md](inbox-followups.md).

## Version string

The v0.1.0-alpha.1 binaries were built from `main` before `da version` existed. `da --version` on that build exits 2. `da doctor` prints `"version": "0.1.0"`. The app plist short version is `0.1.0-alpha.1`.

A later tag stamps the label with `-ldflags` and `da version` prints `version` `0.1.0` plus `release`, `commit`, and `date`. Plugin compatibility keeps using `version`, because a pre-release tag is not valid plugin semver.

## Known issues

The chromeless window needs Chrome or Edge. On Linux, Chromium counts. If none of those is installed, DAP opens the default browser (`open`, `xdg-open`, or `cmd /c start`).

Two `da` processes cannot use the same data directory at once.

## Cutting the next tag

Pushing a `v*` tag runs `.github/workflows/release.yml` once that workflow is on the default branch. The tag `v0.1.0-alpha.1` points at `546a2e1`, which does not contain the workflow, so Actions does not rebuild that release.

The workflow cross-compiles on `ubuntu-latest`:

- macOS arm64, amd64, and a universal binary (`lipo`), ad-hoc signed with `rcodesign` (signature flag `ADHOC`, same outcome as `codesign -s -`)
- Linux amd64 and arm64 tarballs, plus `.deb` packages
- a Windows amd64 zip with `da.exe` and `DAP.cmd`

Tags that contain `-alpha`, `-beta`, or `-rc` are marked pre-release. Nothing is notarized.

Local equivalent, without uploading:

```bash
packaging/release.sh v0.1.0-alpha.2
```

Artifacts land in `dist/release/`. On a Mac the macOS step uses `lipo` and `codesign -s -` when those tools are on `PATH`. The `.deb` step needs `dpkg-deb`.

To publish from a checkout that already has the artifacts:

```bash
packaging/macos/publish.sh v0.1.0-alpha.2
```

That command uses the current `gh` login. Do not point it at `v0.1.0-alpha.1`.
