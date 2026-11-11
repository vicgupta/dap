# Install the DAP CLI into the current directory.
# In PowerShell:
#   Invoke-WebRequest -Uri https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.ps1 -OutFile .\install.ps1
#   powershell -ExecutionPolicy Bypass -Scope Process -File .\install.ps1
#   .\da.exe run
# Does not change PATH, does not touch HKLM or Program Files, and does not request elevation.
# Downloads and temp files stay in this folder, then the temp files are removed.
# Data stays in .\.dap when you run da.exe.
# Override the directory with -InstallDir or $env:DAP_INSTALL_DIR.
# Tests can point DAP_ASSET_DIR at a folder of zips, or DAP_RELEASE_BASE at a URL prefix.
param(
  [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"
# Windows PowerShell 5.1's progress bar makes Invoke-WebRequest stall or fail.
$ProgressPreference = "SilentlyContinue"
try {
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
} catch {
}

$Repo = if ($env:DAP_REPO) { $env:DAP_REPO } else { "vicgupta/dap" }
# Newest GitHub release, including pre-releases (/releases/latest skips those).
# FallbackTag is used when that API does not answer. DAP_TAG overrides both.
$FallbackTag = if ($env:DAP_TAG) { $env:DAP_TAG } else { "v0.1.0-alpha.4" }

function Get-DapArch {
  $arch = $env:PROCESSOR_ARCHITECTURE
  if ($env:PROCESSOR_ARCHITEW6432) { $arch = $env:PROCESSOR_ARCHITEW6432 }
  switch ($arch) {
    "AMD64" { return "amd64" }
    "ARM64" { return "arm64" }
    default { Write-Error "Unsupported CPU: $arch. Windows builds are amd64 and arm64." }
  }
}

function Get-DapTag {
  if ($env:DAP_ASSET_DIR) { return "local" }
  if ($env:DAP_TAG) { return $env:DAP_TAG }
  try {
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=1" -Headers @{ Accept = "application/vnd.github+json" } -UseBasicParsing -TimeoutSec 30
    if ($rel -is [System.Array]) { $rel = $rel[0] }
    if ($rel.tag_name) { return [string]$rel.tag_name }
  } catch {
    Write-Host "Could not reach the GitHub releases API ($($_.Exception.Message)). Using $FallbackTag."
  }
  return $FallbackTag
}

function Save-DapFile($name, $destFile) {
  if ($env:DAP_ASSET_DIR) {
    $src = Join-Path $env:DAP_ASSET_DIR $name
    if (-not (Test-Path -LiteralPath $src)) {
      Write-Error "Could not find $name in $env:DAP_ASSET_DIR."
    }
    Copy-Item -LiteralPath $src -Destination $destFile -Force
    return
  }
  $root = if ($env:DAP_RELEASE_BASE) { $env:DAP_RELEASE_BASE.TrimEnd("/") } else { "https://github.com/$Repo/releases/download/$Tag" }
  $url = "$root/$name"
  try {
    Invoke-WebRequest -Uri $url -OutFile $destFile -UseBasicParsing -TimeoutSec 60
  } catch {
    Write-Error "Could not download $url. Check the network and try again. $($_.Exception.Message)"
  }
  if (-not (Test-Path -LiteralPath $destFile) -or (Get-Item -LiteralPath $destFile).Length -eq 0) {
    Write-Error "Could not download $url. The file was empty."
  }
}

function Find-DapZip($checksumText, $goarch, $ver) {
  $names = @("da_windows_$goarch.zip", "da-$ver-windows-$goarch.zip")
  foreach ($name in $names) {
    foreach ($raw in ($checksumText -split "`n")) {
      $line = $raw.Trim()
      if ($line -match '^([0-9a-fA-F]+)\s+(\S+)$' -and $Matches[2] -eq $name) {
        return @{ Name = $name; Hash = $Matches[1] }
      }
    }
  }
  return $null
}

if ($InstallDir) {
  $dest = $InstallDir
} elseif ($env:DAP_INSTALL_DIR) {
  $dest = $env:DAP_INSTALL_DIR
} else {
  $dest = (Get-Location).Path
}
New-Item -ItemType Directory -Force -Path $dest | Out-Null

$goarch = Get-DapArch
$Tag = Get-DapTag
$Ver = $Tag.TrimStart("v")

$work = Join-Path $dest (".install-tmp-" + [guid]::NewGuid().ToString("n"))
New-Item -ItemType Directory -Force -Path $work | Out-Null

try {
  Write-Host "Downloading $Tag (windows $goarch)"
  $sumPath = Join-Path $work "checksums.txt"
  Save-DapFile "checksums.txt" $sumPath
  $sumText = [System.IO.File]::ReadAllText($sumPath)
  $picked = Find-DapZip $sumText $goarch $Ver
  if (-not $picked) {
    Write-Error "Release $Tag has no Windows zip for $goarch. Expected da_windows_$goarch.zip."
  }
  $zipName = $picked.Name
  $zipPath = Join-Path $work $zipName
  Save-DapFile $zipName $zipPath

  $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $zipPath).Hash
  if ($got.ToLower() -ne $picked.Hash.ToLower()) {
    Write-Error "SHA256 mismatch for $zipName`n  expected $($picked.Hash)`n  got      $got"
  }
  Write-Host "SHA256 ok  $zipName"

  if ($env:OS -eq "Windows_NT" -and (Get-Command Unblock-File -ErrorAction SilentlyContinue)) {
    Unblock-File -LiteralPath $zipPath
  }
  # Windows PowerShell 5.1 refuses to expand a zip into a parent of the zip.
  # Expand into the temp directory, then leave only da.exe in the install folder.
  $unpack = Join-Path $work "out"
  New-Item -ItemType Directory -Force -Path $unpack | Out-Null
  Expand-Archive -LiteralPath $zipPath -DestinationPath $unpack -Force
  $unpacked = Join-Path $unpack "da.exe"
  if (-not (Test-Path -LiteralPath $unpacked)) {
    Write-Error "Archive $zipName does not contain da.exe at its root."
  }
  $exe = Join-Path $dest "da.exe"
  Copy-Item -LiteralPath $unpacked -Destination $exe -Force
  if (-not (Test-Path -LiteralPath $exe)) {
    Write-Error "Archive $zipName does not contain da.exe at its root."
  }
  if ($env:OS -eq "Windows_NT" -and (Get-Command Unblock-File -ErrorAction SilentlyContinue)) {
    Unblock-File -LiteralPath $exe
  }

  Write-Host "Installed $exe"
  Write-Host "Data stays in .\.dap in this folder."
  Write-Host "Run: .\da.exe run"
} finally {
  Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
