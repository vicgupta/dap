# Install da.exe from local Windows zips and smoke-test it.
# Run this on windows-latest with Windows PowerShell 5.1 and with PowerShell 7.
# DAP_ASSET_DIR is the directory that contains da_windows_*.zip.
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if (-not $env:DAP_ASSET_DIR) {
  $env:DAP_ASSET_DIR = Join-Path $root "dist\windows-cli"
}
$assets = $env:DAP_ASSET_DIR
$installer = Join-Path $root "scripts\install.ps1"
if (-not (Test-Path -LiteralPath $installer)) {
  throw "missing $installer"
}
$zip = Join-Path $assets "da_windows_amd64.zip"
if (-not (Test-Path -LiteralPath $zip)) {
  throw "missing $zip"
}

function Write-Checksums($dir) {
  $names = @("da_windows_amd64.zip", "da_windows_arm64.zip", "checksums.txt")
  $lines = New-Object System.Collections.Generic.List[string]
  foreach ($name in @("da_windows_amd64.zip", "da_windows_arm64.zip")) {
    $path = Join-Path $dir $name
    if (-not (Test-Path -LiteralPath $path)) { continue }
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLower()
    $lines.Add("$hash  $name")
  }
  $out = Join-Path $dir "checksums.txt"
  $utf8 = New-Object System.Text.UTF8Encoding $false
  [System.IO.File]::WriteAllLines($out, $lines.ToArray(), $utf8)
}

function Assert-Clean($dir) {
  $names = @(Get-ChildItem -Force -LiteralPath $dir | ForEach-Object { $_.Name })
  foreach ($name in $names) {
    if ($name -ne "da.exe" -and $name -ne "install.ps1" -and $name -ne ".dap") {
      throw "unexpected file in ${dir}: $name"
    }
  }
  if ($names -notcontains "da.exe") { throw "da.exe was not installed in $dir" }
  $tmp = @(Get-ChildItem -Force -LiteralPath $dir -Filter ".install-tmp-*" -ErrorAction SilentlyContinue)
  if ($tmp.Count -gt 0) { throw "temp directory was left in $dir" }
}

Write-Checksums $assets

Write-Host "PSVersion $($PSVersionTable.PSVersion)"

# A bad checksum must fail before da.exe is written.
$badDir = Join-Path $assets "bad"
New-Item -ItemType Directory -Force -Path $badDir | Out-Null
Copy-Item -LiteralPath $zip -Destination (Join-Path $badDir "da_windows_amd64.zip") -Force
$utf8 = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText((Join-Path $badDir "checksums.txt"), "deadbeef  da_windows_amd64.zip`n", $utf8)
$badInstall = Join-Path ([System.IO.Path]::GetFullPath($assets)) "bad-install"
if (Test-Path $badInstall) { Remove-Item -Recurse -Force $badInstall }
New-Item -ItemType Directory -Force -Path $badInstall | Out-Null
$prevAsset = $env:DAP_ASSET_DIR
$env:DAP_ASSET_DIR = $badDir
$failed = $false
try {
  & $installer -InstallDir $badInstall
} catch {
  $failed = $true
  if ("$_" -notmatch "SHA256 mismatch") {
    throw "expected a checksum error, got: $_"
  }
  Write-Host "checksum error ok"
}
$env:DAP_ASSET_DIR = $prevAsset
if (-not $failed) { throw "bad checksum was accepted" }
if (Test-Path (Join-Path $badInstall "da.exe")) { throw "da.exe was written after a bad checksum" }

# Mark-of-the-Web on the zip. Expand-Archive on a desktop must still work.
$zoneZip = Join-Path $assets "da_windows_amd64.zip"
try {
  Set-Content -LiteralPath $zoneZip -Stream Zone.Identifier -Value "[ZoneTransfer]`r`nZoneId=3"
} catch {
  Write-Host "could not mark the zip: $($_.Exception.Message)"
}

$localBefore = @()
if ($env:LOCALAPPDATA -and (Test-Path $env:LOCALAPPDATA)) {
  $localBefore = @(Get-ChildItem -Force $env:LOCALAPPDATA | ForEach-Object { $_.Name })
}

$legacy = Join-Path $env:USERPROFILE ".dap"
$legacyBefore = Test-Path $legacy
$dir = Join-Path $assets "smoke-$($PSVersionTable.PSVersion.Major)"
if (Test-Path $dir) { Remove-Item -Recurse -Force $dir }
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Copy-Item -LiteralPath $installer -Destination (Join-Path $dir "install.ps1") -Force
$env:DAP_TAG = ""
$env:DAP_HOME = ""
$env:DAP_INSTALL_DIR = ""
$env:DAP_RELEASE_BASE = ""
Push-Location $dir
try {
  & (Join-Path $dir "install.ps1")
} finally {
  Pop-Location
}
Assert-Clean $dir

$ver = & (Join-Path $dir "da.exe") --version
Write-Host $ver
if ($ver -notmatch "0\.0\.0-ci") { throw "version stamp missing: $ver" }

$stdout = Join-Path $dir "run.out"
$stderr = Join-Path $dir "run.err"
$p = Start-Process -FilePath (Join-Path $dir "da.exe") -ArgumentList @("run", "--no-browser") -WorkingDirectory $dir -PassThru -WindowStyle Hidden -RedirectStandardOutput $stdout -RedirectStandardError $stderr
$ok = $false
try {
  foreach ($i in 1..40) {
    Start-Sleep -Milliseconds 250
    try {
      $resp = Invoke-WebRequest -Uri "http://127.0.0.1:7391/" -UseBasicParsing
      if ($resp.StatusCode -eq 200) { $ok = $true; break }
    } catch {
    }
    if ($p.HasExited) { break }
  }
  if (-not $ok) {
    Write-Host "---- stdout ----"
    if (Test-Path $stdout) { Get-Content $stdout }
    Write-Host "---- stderr ----"
    if (Test-Path $stderr) { Get-Content $stderr }
    throw "da.exe run --no-browser did not serve http://127.0.0.1:7391/"
  }
  Write-Host "HTTP 200"
} finally {
  if (-not $p.HasExited) {
    try { & taskkill.exe /PID $p.Id /T /F | Out-Null } catch {}
    Start-Sleep -Seconds 1
  }
}

$logged = ""
if (Test-Path $stdout) { $logged = Get-Content $stdout -Raw }
if ($logged -notmatch "127\.0\.0\.1:7391") {
  throw "run did not report 127.0.0.1:7391: $logged"
}
if (-not (Test-Path (Join-Path $dir ".dap"))) {
  throw "run did not create $dir\.dap"
}
$legacy = Join-Path $env:USERPROFILE ".dap"
if (-not $legacyBefore -and (Test-Path $legacy)) {
  throw "run wrote $legacy"
}
Assert-Clean $dir

if ($env:LOCALAPPDATA -and (Test-Path $env:LOCALAPPDATA)) {
  $localAfter = @(Get-ChildItem -Force $env:LOCALAPPDATA | ForEach-Object { $_.Name })
  $added = @()
  foreach ($name in $localAfter) {
    if ($localBefore -notcontains $name) { $added += $name }
  }
  if ($added.Count -gt 0) {
    throw "LOCALAPPDATA changed: $($added -join ', ')"
  }
}

$env:DAP_ASSET_DIR = ""
$env:DAP_TAG = "v0.0.0-ci"
$env:DAP_RELEASE_BASE = "http://127.0.0.1:9"
$netDir = Join-Path $assets "net-fail-$($PSVersionTable.PSVersion.Major)"
New-Item -ItemType Directory -Force -Path $netDir | Out-Null
$failed = $false
try {
  & $installer -InstallDir $netDir
} catch {
  $failed = $true
  $text = "$_"
  if ($text -notmatch "Could not download") {
    throw "expected a network error, got: $text"
  }
  Write-Host "network error ok"
}
if (-not $failed) { throw "network failure was ignored" }
if (Test-Path (Join-Path $netDir "da.exe")) { throw "da.exe was written after a network error" }

Write-Host "smoke ok $($PSVersionTable.PSVersion)"
