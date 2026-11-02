# Install a DAP Windows release from the private GitHub repo.
# One-time setup: gh auth login
# Then, in PowerShell:
#   gh release download v0.1.0-alpha.1 --repo vicgupta/dap --pattern install.ps1
#   powershell -ExecutionPolicy Bypass -File .\install.ps1
# v0.1.0-alpha.1 ships macOS packages only. Later tags ship the Windows zip.
$ErrorActionPreference = "Stop"

$Repo = if ($env:DAP_REPO) { $env:DAP_REPO } else { "vicgupta/dap" }
$Tag = if ($env:DAP_TAG) { $env:DAP_TAG } else { "v0.1.0-alpha.1" }
$Ver = $Tag.TrimStart("v")

if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
  Write-Error "GitHub CLI (gh) is required. Install it, then run: gh auth login"
}
& gh auth status
if ($LASTEXITCODE -ne 0) {
  Write-Error "gh is not logged in. Run: gh auth login"
}

$arch = $env:PROCESSOR_ARCHITECTURE
if ($arch -ne "AMD64") {
  Write-Error "The Windows package is amd64. This machine reports $arch."
}

$zipName = "DAP-$Ver-windows-amd64.zip"
$workdir = Join-Path ([System.IO.Path]::GetTempPath()) ("dap-install-" + [guid]::NewGuid().ToString("n"))
New-Item -ItemType Directory -Path $workdir | Out-Null

try {
  Write-Host "Downloading $Tag (windows amd64) from $Repo"
  & gh release download $Tag --repo $Repo --dir $workdir --pattern checksums.txt --pattern $zipName
  if ($LASTEXITCODE -ne 0 -or -not (Test-Path (Join-Path $workdir $zipName))) {
    Write-Error "Could not download $zipName from $Tag. v0.1.0-alpha.1 includes macOS packages only. The Windows zip is built for a later tag."
  }

  $expected = $null
  foreach ($line in Get-Content (Join-Path $workdir "checksums.txt")) {
    if ($line -match '^([0-9a-fA-F]+)\s+(\S+)$' -and $Matches[2] -eq $zipName) {
      $expected = $Matches[1]
      break
    }
  }
  if (-not $expected) {
    Write-Error "No SHA256 line for $zipName in checksums.txt"
  }
  $got = (Get-FileHash -Algorithm SHA256 (Join-Path $workdir $zipName)).Hash
  if ($got.ToLower() -ne $expected.ToLower()) {
    Write-Error "SHA256 mismatch for $zipName`n  expected $expected`n  got      $got"
  }
  Write-Host "SHA256 ok  $zipName"

  $unpack = Join-Path $workdir "unpack"
  Expand-Archive -LiteralPath (Join-Path $workdir $zipName) -DestinationPath $unpack -Force

  $dest = Join-Path $env:LOCALAPPDATA "DAP"
  New-Item -ItemType Directory -Force -Path $dest | Out-Null
  Copy-Item -Force (Join-Path $unpack "da.exe") (Join-Path $dest "da.exe")
  Copy-Item -Force (Join-Path $unpack "DAP.cmd") (Join-Path $dest "DAP.cmd")
  if (Test-Path (Join-Path $dest "plugins")) {
    Remove-Item -Recurse -Force (Join-Path $dest "plugins")
  }
  Copy-Item -Recurse -Force (Join-Path $unpack "plugins") (Join-Path $dest "plugins")

  Unblock-File -LiteralPath (Join-Path $dest "da.exe")
  Unblock-File -LiteralPath (Join-Path $dest "DAP.cmd")
  Get-ChildItem -Recurse -File (Join-Path $dest "plugins") | Unblock-File

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if (-not $userPath) { $userPath = "" }
  $parts = @($userPath.Split(";") | Where-Object { $_ -and ($_.TrimEnd("\") -ne $dest.TrimEnd("\")) })
  $joined = $dest
  foreach ($p in $parts) { $joined = "$joined;$p" }
  [Environment]::SetEnvironmentVariable("Path", $joined, "User")
  if ((";" + $env:Path + ";") -notlike "*;$dest;*") {
    $env:Path = "$dest;" + $env:Path
  }

  $menu = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
  New-Item -ItemType Directory -Force -Path $menu | Out-Null
  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut((Join-Path $menu "DAP.lnk"))
  $shortcut.TargetPath = Join-Path $dest "da.exe"
  $shortcut.Arguments = "run"
  $shortcut.WorkingDirectory = $dest
  $shortcut.Description = "DAP"
  $shortcut.Save()

  Write-Host ""
  Write-Host "Installed da.exe and DAP.cmd at $dest"
  Write-Host "Start menu shortcut: DAP (runs da.exe run)"
  Write-Host "Data directory: $env:USERPROFILE\.dap  (or DAP_HOME)"
  Write-Host ""
  Write-Host "SmartScreen can still block an unsigned exe the first time:"
  Write-Host "  Windows protected your PC -> More info -> Run anyway"
  Write-Host ""
  Write-Host "Example plugins need Python (python or py):"
  Write-Host "  da plugin install `"$dest\plugins\mail-fake`" --approve"
  Write-Host "  da plugin install `"$dest\plugins\inbox-followups`" --approve"
  Write-Host "  da plugin install `"$dest\plugins\hello-watch`" --approve"
  Write-Host "  da plugin install `"$dest\plugins\workflow`" --approve"
  Write-Host "Gmail and Outlook also need a client id. See docs/inbox-followups.md in the repo."
}
finally {
  Remove-Item -Recurse -Force $workdir -ErrorAction SilentlyContinue
}
