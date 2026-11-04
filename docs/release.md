# Releases

v0.1.0-alpha.1 is a private GitHub pre-release built from `main` `546a2e1`. That tag is macOS only: Apple Silicon, Intel, and a universal binary, ad-hoc signed and not notarized. It does not include Linux or Windows packages, and this repo does not replace that tag.

The release workflow on this branch, once it is on `main`, builds macOS, Linux, and Windows for a later `v*` tag. Themes (PR #4) and the plugin creator (PR #5) are not in v0.1.0-alpha.1.

Downloads are public HTTPS. `install.sh` and `install.ps1` use `curl` or `Invoke-WebRequest` against `https://github.com/vicgupta/dap/releases/download/<tag>/`. They do not use the `gh` CLI. `/releases/latest/download/` skips pre-releases, so the scripts use the tag. They copy `da` into the current folder, check SHA256, and keep data in `./.dap` in that folder.

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

v0.1.0-alpha.4 ships the macOS `da` tarballs and Windows `da.exe` zips (`da-<version>-windows-amd64.zip` and `da-<version>-windows-arm64.zip`, each containing only `da.exe`). It does not ship Linux archives or `DAP.app`.

## Later tags: Linux and Windows

`packaging/release.sh` and `.github/workflows/release.yml` add these archives next to the macOS files. `checksums.txt` covers every file. Pre-release tags (`-alpha`, `-beta`, `-rc`) stay marked pre-release.

| Asset | What it is |
| --- | --- |
| `da-<version>-linux-amd64.tar.gz` | `da`, the `dap` launcher, and `plugins/` |
| `da-<version>-linux-arm64.tar.gz` | Same for arm64 |
| `dap_<debver>_amd64.deb` | `/usr/bin/da`, `/usr/bin/dap`, `/usr/share/dap/plugins` |
| `dap_<debver>_arm64.deb` | Same for arm64 |
| `da-<version>-windows-amd64.zip` | `da.exe` only |
| `da-<version>-windows-arm64.zip` | `da.exe` only |
| `DAP-<version>-windows-amd64.zip` | `da.exe`, `DAP.cmd`, and `plugins\` |
| `install.sh` | macOS and Linux CLI, into the current directory |
| `install.ps1` | Windows CLI, into the current directory |

`<version>` is the tag without `v`. `<debver>` is the same string with the first hyphen replaced by `~`, so `0.1.0-alpha.2` becomes `0.1.0~alpha.2`. There is no AppImage.

### Linux

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.sh
sh install.sh
./da run
```

On Linux, `install.sh` copies `da` into the current directory. It does not change `PATH`. Run `./da run` from that directory. Data stays in `./.dap`. The archive also contains `dap` and `plugins/` if you want to copy those yourself, still inside this folder. `dap` runs `da run`, and both commands have to stay in the same directory.

The `.deb` packages are built for a system install. `install.sh` does not use them. The install that stays in this folder is the script above.

`dap` opens the URL in the default browser (`xdg-open` on Linux). Chrome is not required. `--no-browser` only prints the URL.

### Windows

```powershell
Invoke-WebRequest -Uri https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.ps1 -OutFile .\install.ps1
powershell -ExecutionPolicy Bypass -Scope Process -File .\install.ps1
.\da.exe run
```

The script copies `da.exe` into the current directory (a normal user folder; not Program Files), runs `Unblock-File` on that copy, and does not change the user or system `PATH`. It does not write HKLM, install a service, or ask for elevation. Data stays in `.\.dap` in that folder, or in a relative `%DAP_HOME%`.

### SmartScreen

The Windows exe is unsigned. There is no Authenticode certificate. After a browser download, Windows may say **Windows protected your PC**.

Choose **More info**, then **Run anyway**.

In PowerShell, from the folder that contains the exe:

```powershell
Unblock-File .\da.exe
```

`install.ps1` already unblocks the copy it writes. A browser download can still mark the zip with a Mark-of-the-Web (`Zone.Identifier`). SmartScreen can show **Windows protected your PC** the first time an unsigned exe runs. **More info**, then **Run anyway**, does not grant administrator rights. DAP opens the default browser. Chrome and Edge are not required.

## After install

Example plugins need Python (`python3` on macOS and Linux; `python` or `py` on Windows).

```bash
./da plugin install ./plugins/mail-fake --approve
./da plugin install ./plugins/inbox-followups --approve
./da plugin install ./plugins/hello-watch --approve
./da plugin install ./plugins/workflow --approve
./da run
```

The CLI installer does not copy example plugins. Use a checkout of this repo, the plugins inside a macOS app zip, or the `plugins/` directory from a Linux archive. Gmail and Outlook connect through Composio: save the API key on Connections, then press Connect. The steps are in [composio.md](composio.md). The demo mailbox (`mail-fake`) does not need a key. Nothing sends until you approve a card. Direct Microsoft Graph, and legacy direct Gmail, are documented in [inbox-followups.md](inbox-followups.md).

## Version string

The v0.1.0-alpha.1 binaries were built from `main` before `da version` existed. `da --version` on that build exits 2. `da doctor` prints `"version": "0.1.0"`. The app plist short version is `0.1.0-alpha.1`.

A later tag stamps the label with `-ldflags` and `da version` prints `version` `0.1.0` plus `release`, `commit`, and `date`. Plugin compatibility keeps using `version`, because a pre-release tag is not valid plugin semver.

## Known issues

DAP opens the default browser (`open` on macOS, `rundll32 url.dll,FileProtocolHandler` on Windows, `xdg-open` on Linux). A machine with only Firefox, Brave, or Safari still gets the UI. `da run --no-browser` prints the URL and does not launch anything.

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
