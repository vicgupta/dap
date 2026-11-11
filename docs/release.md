# Releases

v0.1.0-alpha.1 is a private GitHub pre-release built from `main` `546a2e1`. That tag is macOS only: Apple Silicon, Intel, and a universal binary, ad-hoc signed and not notarized. It does not include Linux or Windows packages, and this repo does not replace that tag.

The release workflow on `main` builds macOS, Linux, and Windows CLI binaries for a `v*` tag. Each archive contains only `da` (`da.exe` on Windows). It does not publish `DAP.app`, `.deb` packages, or launcher and plugins bundles. Themes (PR #4) and the plugin creator (PR #5) are not in v0.1.0-alpha.1.

Downloads are public HTTPS. `install.sh` and `install.ps1` ask `https://api.github.com/repos/vicgupta/dap/releases?per_page=1` for the newest release, including pre-releases, then download that tag from `https://github.com/vicgupta/dap/releases/download/<tag>/`. They do not use the `gh` CLI. `/releases/latest/download/` skips pre-releases, so the scripts do not use it. If the API does not answer, they fall back to `v0.1.0-alpha.4`. `DAP_TAG` overrides both. They copy `da` into the current folder, check SHA256, and keep data in `./.dap` in that folder.

## v0.1.0-alpha.1 (macOS)

Release page: <https://github.com/vicgupta/dap/releases/tag/v0.1.0-alpha.1>

That tag's installer is not the current one. Current installs use the commands in the README. They copy `da` into the folder you run them from and keep data in `./.dap`.

v0.1.0-alpha.1 shipped a macOS app and CLI with this layout:

| Piece | Location |
| --- | --- |
| `DAP.app` | inside the downloaded zip |
| `da` | inside the downloaded archive |

Direct download of one file, into the current folder:

```bash
curl -fsSL -o checksums.txt https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/checksums.txt
curl -fsSL -o DAP-0.1.0-alpha.1-macos-arm64.zip https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/DAP-0.1.0-alpha.1-macos-arm64.zip
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

## Installer

`install.sh` and `install.ps1` copy the `da` CLI into the directory where you run them (or `DAP_INSTALL_DIR`, `--dir`, or `-InstallDir`). They check SHA256, do not change `PATH`, and do not use sudo or an administrator prompt. On macOS, `install.sh` runs `xattr -d com.apple.quarantine` on the copied binary. On Windows, `install.ps1` runs `Unblock-File` on `da.exe`.

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.sh
sh install.sh
./da run
```

```powershell
Invoke-WebRequest -Uri https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.ps1 -OutFile .\install.ps1
powershell -ExecutionPolicy Bypass -Scope Process -File .\install.ps1
.\da.exe run
```

`-Scope Process` changes execution policy only for that PowerShell process. Data stays in `./.dap` in that folder. A relative `$DAP_HOME` is resolved from the folder. The scripts do not change `PATH`.

v0.1.0-alpha.4 is the installer fallback when the GitHub releases API does not answer. Published tags through v0.1.0-alpha.5 ship macOS `da` tarballs and Windows `da.exe` zips (`da-<version>-windows-amd64.zip` and `da-<version>-windows-arm64.zip`, each containing only `da.exe`). They do not ship Linux archives. On Linux, `install.sh` stops if the selected release has no `da-<version>-linux-<arch>.tar.gz` asset. Set `DAP_TAG` to a release that includes those files. The next tag built by this workflow publishes the Linux tarballs below.

## Release assets

`packaging/release.sh` and `.github/workflows/release.yml` publish this set and nothing else. `checksums.txt` covers every file. Pre-release tags (`-alpha`, `-beta`, `-rc`) stay marked pre-release.

| Asset | What it is |
| --- | --- |
| `da-<version>-darwin-arm64.tar.gz` | Apple Silicon `da` |
| `da-<version>-darwin-amd64.tar.gz` | Intel `da` |
| `da-<version>-darwin-universal.tar.gz` | Fat `da` |
| `da-<version>-linux-amd64.tar.gz` | Linux x86_64 `da` |
| `da-<version>-linux-arm64.tar.gz` | Linux arm64 `da` |
| `da_windows_amd64.zip` | `da.exe` only, stable name |
| `da_windows_arm64.zip` | `da.exe` only, stable name |
| `da-<version>-windows-amd64.zip` | same zip, versioned name |
| `da-<version>-windows-arm64.zip` | same zip, versioned name |
| `checksums.txt` | SHA256 |
| `install.sh` | macOS and Linux CLI, into the current directory |
| `install.ps1` | Windows CLI, into the current directory |

`<version>` is the tag without `v`. Each archive contains only that one binary. There is no `DAP.app`, no `.deb`, and no AppImage.

### Linux

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.sh
sh install.sh
./da run
```

On Linux, `install.sh` copies `da` into the current directory. It does not change `PATH` and does not use sudo. `uname -m` maps `x86_64` to amd64 and `aarch64` or `arm64` to arm64. SHA256 is checked with `sha256sum` (macOS uses `shasum`). Run `./da run` from that directory. Data stays in `./.dap`.

`da run` opens the URL in the default browser (`xdg-open`). Chrome is not required. On a machine with no desktop or browser (a server, or WSL without a browser), run `./da run --no-browser` and open http://127.0.0.1:7391.

### Windows

```powershell
Invoke-WebRequest -Uri https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.ps1 -OutFile .\install.ps1
powershell -ExecutionPolicy Bypass -Scope Process -File .\install.ps1
.\da.exe run
```

`install.ps1` picks amd64 or arm64, downloads `da_windows_amd64.zip` or `da_windows_arm64.zip` from the newest public release, checks SHA256, and expands that zip so `.\da.exe` is in the current folder. Temp files stay in that folder and are removed. It does not change PATH, write HKLM, or ask for elevation. Data stays in `.\.dap` when you run `da.exe`.

To install without the script, download `da_windows_amd64.zip` or `da_windows_arm64.zip` from the [releases page](https://github.com/vicgupta/dap/releases), choose Extract All, and run `da.exe`. `/releases/latest` skips pre-releases, so use the newest release in that list. The zip contains only `da.exe`.

### SmartScreen

The Windows exe is unsigned. There is no Authenticode certificate. After a browser download, Windows may say **Windows protected your PC**.

Choose **More info**, then **Run anyway**.

In PowerShell, from the folder that contains the exe:

```powershell
Unblock-File .\da.exe
```

`install.ps1` unblocks the downloaded zip before `Expand-Archive`, then unblocks `da.exe`. A browser download can still mark the zip with a Mark-of-the-Web (`Zone.Identifier`). SmartScreen can show **Windows protected your PC** the first time an unsigned exe runs. **More info**, then **Run anyway**, does not grant administrator rights. DAP opens the default browser. Chrome and Edge are not required.

## After install

Example plugins need Python (`python3` on macOS and Linux; `python` or `py` on Windows).

```bash
./da plugin install ./plugins/mail-fake --approve
./da plugin install ./plugins/inbox-followups --approve
./da plugin install ./plugins/hello-watch --approve
./da plugin install ./plugins/workflow --approve
./da run
```

The CLI installer does not copy example plugins. Use a checkout of this repo. Gmail and Outlook connect through Composio: save the API key on Connections, then press Connect. The steps are in [composio.md](composio.md). The demo mailbox (`mail-fake`) does not need a key. Nothing sends until you approve a card. Direct Microsoft Graph, and legacy direct Gmail, are documented in [inbox-followups.md](inbox-followups.md).

## Version string

The v0.1.0-alpha.1 binaries were built from `main` before `da version` existed. `da --version` on that build exits 2. `da doctor` prints `"version": "0.1.0"`. The app plist short version is `0.1.0-alpha.1`.

A later tag stamps the label with `-ldflags` and `da version` prints `version` `0.1.0` plus `release`, `commit`, and `date`. Plugin compatibility keeps using `version`, because a pre-release tag is not valid plugin semver.

## Known issues

DAP opens the default browser (`open` on macOS, `rundll32 url.dll,FileProtocolHandler` on Windows, `xdg-open` on Linux). A machine with only Firefox, Brave, or Safari still gets the UI. `da run --no-browser` prints the URL and does not launch anything.

Two `da` processes cannot use the same data directory at once.

## Cutting the next tag

Pushing a `v*` tag runs `.github/workflows/release.yml` once that workflow is on the default branch. The tag `v0.1.0-alpha.1` points at `546a2e1`, which does not contain the workflow, so Actions does not rebuild that release.

The workflow cross-compiles on `ubuntu-latest` and uploads CLI binaries only:

- macOS arm64, amd64, and a universal `da` tarball (`lipo`), ad-hoc signed with `rcodesign` (signature flag `ADHOC`, same outcome as `codesign -s -`)
- Linux amd64 and arm64 `da` tarballs (`CGO_ENABLED=0`), each containing only `da`
- Windows amd64 and arm64 zips that contain only `da.exe` (`da_windows_amd64.zip` and `da_windows_arm64.zip`, plus the same bytes under `da-<version>-windows-*.zip`)

Tags that contain `-alpha`, `-beta`, or `-rc` are marked pre-release. Nothing is notarized. `DAP.app`, `.deb` packages, and launcher or plugins bundles are not uploaded.

Local equivalent, without uploading:

```bash
packaging/release.sh v0.1.0-alpha.2
```

Artifacts land in `dist/release/`. On a Mac the macOS step uses `lipo` and `codesign -s -` when those tools are on `PATH`.

To publish from a checkout that already has the artifacts:

```bash
packaging/macos/publish.sh v0.1.0-alpha.2
```

That command uses the current `gh` login. Do not point it at `v0.1.0-alpha.1`.
