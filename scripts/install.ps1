# Install the DAP CLI into the current directory.
# In PowerShell:
#   Invoke-WebRequest -Uri https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.ps1 -OutFile .\install.ps1
#   powershell -ExecutionPolicy Bypass -Scope Process -File .\install.ps1
#   .\da.exe run
# Does not change PATH, does not touch HKLM or Program Files, and does not request elevation.
# Override the directory with -InstallDir or $env:DAP_INSTALL_DIR.
# Downloads and da.exe stay in that folder. Data stays in .\.dap there.
param(
  [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"

$Repo = if ($env:DAP_REPO) { $env:DAP_REPO } else { "vicgupta/dap" }
# Newest GitHub release, including pre-releases (/releases/latest skips those).
# FallbackTag is used when that API does not answer. DAP_TAG overrides both.
$FallbackTag = if ($env:DAP_TAG) { $env:DAP_TAG } else { "v0.1.0-alpha.4" }
if ($env:DAP_TAG) {
  $Tag = $env:DAP_TAG
} else {
  $Tag = $FallbackTag
  try {
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=1" -Headers @{ Accept = "application/vnd.github+json" } -UseBasicParsing
    if ($rel -is [System.Array]) { $rel = $rel[0] }
    if ($rel.tag_name) { $Tag = [string]$rel.tag_name }
  } catch {
    $Tag = $FallbackTag
  }
}
$Ver = $Tag.TrimStart("v")

if ($InstallDir) {
  $dest = $InstallDir
} elseif ($env:DAP_INSTALL_DIR) {
  $dest = $env:DAP_INSTALL_DIR
} else {
  $dest = (Get-Location).Path
}

$arch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) { $arch = $env:PROCESSOR_ARCHITEW6432 }
switch ($arch) {
  "AMD64" { $goarch = "amd64" }
  "ARM64" { $goarch = "arm64" }
  default { Write-Error "Unsupported CPU: $arch. This release ships windows/amd64 and windows/arm64." }
}

$zipName = "da-$Ver-windows-$goarch.zip"
$workdir = Join-Path $dest ".dap\tmp\install-$([guid]::NewGuid().ToString('n'))"
New-Item -ItemType Directory -Path $workdir -Force | Out-Null

try {
  $base = "https://github.com/$Repo/releases/download/$Tag"
  Write-Host "Downloading $Tag (windows $goarch) from $base"
  Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $workdir "checksums.txt") -UseBasicParsing
  Invoke-WebRequest -Uri "$base/$zipName" -OutFile (Join-Path $workdir $zipName) -UseBasicParsing
  if (-not (Test-Path (Join-Path $workdir $zipName))) {
    Write-Error "Could not download $zipName from $Tag."
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
  $src = Join-Path $unpack "da.exe"
  if (-not (Test-Path -LiteralPath $src)) {
    Write-Error "Archive $zipName does not contain da.exe."
  }

  New-Item -ItemType Directory -Force -Path $dest | Out-Null
  $exe = Join-Path $dest "da.exe"
  Copy-Item -LiteralPath $src -Destination $exe -Force
  # Windows PowerShell 5.1 and PowerShell 7 on Windows set OS=Windows_NT.
  # Unblock-File removes the Mark-of-the-Web. It is not a Linux cmdlet.
  if ($env:OS -eq "Windows_NT") {
    Unblock-File -LiteralPath $exe
  }

  Write-Host ""
  Write-Host "Installed da.exe at $exe"
  if ($dest -eq (Get-Location).Path) {
    Write-Host "Run it from this folder:"
    Write-Host "  .\da.exe run"
  } else {
    Write-Host "Run it with:"
    Write-Host "  & '$exe' run"
  }
  Write-Host "Data stays in .\.dap in the folder where you run da.exe."
  if ($env:DAP_HOME) {
    Write-Host "DAP_HOME is set ($env:DAP_HOME). A relative value is resolved from that folder."
  }
  Write-Host ""
  Write-Host "This installer does not change PATH and does not need administrator rights."
  Write-Host ""
  Write-Host "da.exe is unsigned. On Windows, Unblock-File cleared the download mark on this copy."
  Write-Host "If Windows says 'Windows protected your PC', choose More info, then Run anyway. That is not an administrator prompt."
  Write-Host "da.exe run opens the URL in the default browser. Chrome is not required. da.exe run --no-browser only prints the URL."
}
finally {
  Remove-Item -Recurse -Force $workdir -ErrorAction SilentlyContinue
}
