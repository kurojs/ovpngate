$ErrorActionPreference = "Stop"

$Repo   = "kurojs/ovpngate"
$AppDir = Join-Path $env:LOCALAPPDATA "ovpngate"
$Exe    = Join-Path $AppDir "ovpngate.exe"

Write-Host "==> ovpngate installer" -ForegroundColor Cyan

try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "ovpngate-installer" }
    $version = $release.tag_name.TrimStart("v")
    Write-Host "==> Latest version: v$version" -ForegroundColor Green
} catch {
    Write-Host "==> ERROR: could not query GitHub releases: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

if (Test-Path $Exe) {
    try {
        $current = & $Exe --version 2>&1 | Out-String
        if ($current -match "\d+\.\d+\.\d+") {
            $installedVersion = $Matches[0]
            if ($installedVersion -eq $version) {
                Write-Host "==> ovpngate already at v$version 窶・nothing to do." -ForegroundColor Green
                exit 0
            } else {
                Write-Host "==> Updating ovpngate v$installedVersion -> v$version" -ForegroundColor Yellow
            }
        }
    } catch {
        Write-Host "==> Could not query installed version, will reinstall." -ForegroundColor Yellow
    }
}

$assetUrl = "https://github.com/$Repo/releases/download/v$version/ovpngate-windows-amd64.exe"
$tmp = Join-Path $env:TEMP "ovpngate-download.exe"
Write-Host "==> Downloading v$version ..." -ForegroundColor Cyan
try {
    Invoke-WebRequest -Uri $assetUrl -OutFile $tmp -UseBasicParsing
} catch {
    Write-Host "==> ERROR: download failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

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