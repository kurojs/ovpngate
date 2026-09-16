# ovpngate installer / updater for Windows
# Usage:
#   Install/update:  powershell -ExecutionPolicy Bypass -c "irm https://raw.githubusercontent.com/kurojs/ovpngate/main/install.ps1 | iex"
#   Or run locally:  powershell -ExecutionPolicy Bypass -File install.ps1

$ErrorActionPreference = "Stop"

$Repo   = "kurojs/ovpngate"
$AppDir = Join-Path $env:LOCALAPPDATA "ovpngate"
$Exe    = Join-Path $AppDir "ovpngate.exe"

Write-Host "==> ovpngate installer" -ForegroundColor Cyan

# 1. Find the latest version from GitHub Releases
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "ovpngate-installer" }
    $version = $release.tag_name.TrimStart("v")
    Write-Host "==> Latest version: v$version" -ForegroundColor Green
} catch {
    Write-Host "==> ERROR: could not query GitHub releases: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# 2. If already installed and up to date, we're done.
if (Test-Path $Exe) {
    try {
        $current = & $Exe --version 2>&1 | Out-String
        if ($current -match "\d+\.\d+\.\d+") {
            $installedVersion = $Matches[0]
            if ($installedVersion -eq $version) {
                Write-Host "==> ovpngate already at v$version — nothing to do." -ForegroundColor Green
                exit 0
            } else {
                Write-Host "==> Updating ovpngate v$installedVersion -> v$version" -ForegroundColor Yellow
            }
        }
    } catch {
        Write-Host "==> Could not query installed version, will reinstall." -ForegroundColor Yellow
    }
}

# 3. Download the Windows binary from the release assets.
$assetUrl = "https://github.com/$Repo/releases/download/v$version/ovpngate-windows-amd64.exe"
$tmp = Join-Path $env:TEMP "ovpngate-download.exe"
Write-Host "==> Downloading v$version ..." -ForegroundColor Cyan
try {
    Invoke-WebRequest -Uri $assetUrl -OutFile $tmp -UseBasicParsing
} catch {
    Write-Host "==> ERROR: download failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# 4. Install (or replace) into %LOCALAPPDATA%\ovpngate and add to user PATH.
New-Item -ItemType Directory -Force -Path $AppDir | Out-Null
Move-Item -Force $tmp $Exe

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$AppDir*") {
    $newPath = if ([string]::IsNullOrEmpty($userPath)) { $AppDir } else { "$userPath;$AppDir" }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "==> Added ""$AppDir"" to your user PATH (new terminals only)." -ForegroundColor Yellow
} else {
    Write-Host "==> ""$AppDir"" already in PATH." -ForegroundColor Gray
}

Write-Host "==> ovpngate v$version installed at:" -ForegroundColor Green
Write-Host "    $Exe"
Write-Host "==> Open a NEW terminal and run:  ovpngate" -ForegroundColor Cyan