param(
    [string] $ManifestPath = "build/windows/installer/dependencies.windows.json",
    [string] $CacheDir = "$env:TEMP\refleks-deps-cache",
    [string] $InstallerResourcesDir = "build/windows/installer/resources"
)

$ErrorActionPreference = "Stop"

function Resolve-RepoPath([string] $Path) {
    if ([System.IO.Path]::IsPathRooted($Path)) {
        return $Path
    }
    return Join-Path (Get-Location) $Path
}

function Require-File([string] $Path, [string] $Description) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Missing required $Description at $Path"
    }
}

function Copy-RequiredFile([string] $Source, [string] $Destination, [string] $Description) {
    Require-File $Source $Description
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Destination) | Out-Null
    Copy-Item -LiteralPath $Source -Destination $Destination -Force
}

$manifestFile = Resolve-RepoPath $ManifestPath
Require-File $manifestFile "dependency manifest"
$manifest = Get-Content -LiteralPath $manifestFile -Raw | ConvertFrom-Json

$ffmpeg = $manifest.ffmpeg
if (-not $ffmpeg.version -or -not $ffmpeg.sourceRevision -or -not $ffmpeg.archiveUrl -or -not $ffmpeg.archiveSha256) {
    throw "FFmpeg manifest entry must include version, sourceRevision, archiveUrl, and archiveSha256."
}
if ($ffmpeg.license -ne "GPLv3") {
    throw "Unexpected FFmpeg license '$($ffmpeg.license)'. This packaging checklist expects the pinned GPLv3 build."
}

New-Item -ItemType Directory -Force -Path $CacheDir | Out-Null
$archivePath = Join-Path $CacheDir ("ffmpeg-{0}-essentials_build.zip" -f $ffmpeg.version)
if (-not (Test-Path -LiteralPath $archivePath)) {
    Write-Host "Downloading $($ffmpeg.name) $($ffmpeg.version)..."
    Invoke-WebRequest -Uri $ffmpeg.archiveUrl -OutFile $archivePath
}

$actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
if ($actualHash -ne $ffmpeg.archiveSha256.ToLowerInvariant()) {
    throw "FFmpeg archive SHA-256 mismatch. Got $actualHash."
}

$extractDir = Join-Path $CacheDir ("ffmpeg-{0}-extract" -f $ffmpeg.version)
if (Test-Path -LiteralPath $extractDir) {
    Remove-Item -LiteralPath $extractDir -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $extractDir | Out-Null
Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force

$ffmpegExe = Get-ChildItem -LiteralPath $extractDir -Recurse -Filter "ffmpeg.exe" | Select-Object -First 1
$ffprobeExe = Get-ChildItem -LiteralPath $extractDir -Recurse -Filter "ffprobe.exe" | Select-Object -First 1
if (-not $ffmpegExe -or -not $ffprobeExe) {
    throw "FFmpeg archive did not contain both ffmpeg.exe and ffprobe.exe."
}
$packageRoot = $ffmpegExe.Directory.Parent.FullName
$readme = Get-ChildItem -LiteralPath $packageRoot -Filter "README*" | Select-Object -First 1
$license = Get-ChildItem -LiteralPath $packageRoot -Filter "LICENSE*" | Select-Object -First 1
if (-not $readme -or -not $license) {
    throw "FFmpeg archive is missing README or LICENSE artifacts required by the packaging checklist."
}

$resourcesRoot = Resolve-RepoPath $InstallerResourcesDir
$binaryStage = Join-Path $resourcesRoot "ffmpeg"
$noticeStage = Join-Path $resourcesRoot "ffmpeg-notices"
if (Test-Path -LiteralPath $binaryStage) {
    Remove-Item -LiteralPath $binaryStage -Recurse -Force
}
if (Test-Path -LiteralPath $noticeStage) {
    Remove-Item -LiteralPath $noticeStage -Recurse -Force
}
New-Item -ItemType Directory -Force -Path (Join-Path $binaryStage "bin") | Out-Null
New-Item -ItemType Directory -Force -Path $noticeStage | Out-Null

Copy-RequiredFile $ffmpegExe.FullName (Join-Path $binaryStage "bin\ffmpeg.exe") "ffmpeg.exe"
Copy-RequiredFile $ffprobeExe.FullName (Join-Path $binaryStage "bin\ffprobe.exe") "ffprobe.exe"
Copy-RequiredFile $readme.FullName (Join-Path $noticeStage $readme.Name) "FFmpeg README"
Copy-RequiredFile $license.FullName (Join-Path $noticeStage $license.Name) "FFmpeg license"

$versionOutput = & $ffmpegExe.FullName -version 2>&1
$versionOutputText = ($versionOutput | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or -not $versionOutputText) {
    throw "Unable to read FFmpeg version/build configuration."
}

$notice = @"
FFmpeg for RefleK's

Package: $($ffmpeg.name)
Version: $($ffmpeg.version)
Source revision: $($ffmpeg.sourceRevision)
Archive URL: $($ffmpeg.archiveUrl)
Archive SHA-256: $($ffmpeg.archiveSha256)
License: $($ffmpeg.license)
Project source: $($ffmpeg.projectSource)
Download page: $($ffmpeg.downloadPage)

Corresponding source:
$($ffmpeg.correspondingSource)

Packaging note:
This script verifies that the required GPL packaging artifacts are present for the pinned FFmpeg package. It is a packaging checklist, not a legal determination.
"@
$notice | Set-Content -LiteralPath (Join-Path $noticeStage "NOTICE.txt") -Encoding UTF8

$versionFile = @"
FFmpeg package version: $($ffmpeg.version)
Source revision: $($ffmpeg.sourceRevision)
Archive SHA-256: $($ffmpeg.archiveSha256)

ffmpeg -version:
$versionOutputText
"@
$versionFile | Set-Content -LiteralPath (Join-Path $noticeStage "VERSION.txt") -Encoding UTF8

if (Test-Path -LiteralPath (Join-Path $packageRoot "doc")) {
    Copy-Item -LiteralPath (Join-Path $packageRoot "doc") -Destination (Join-Path $noticeStage "doc") -Recurse -Force
}

$requiredArtifacts = @(
    (Join-Path $binaryStage "bin\ffmpeg.exe"),
    (Join-Path $binaryStage "bin\ffprobe.exe"),
    (Join-Path $noticeStage "NOTICE.txt"),
    (Join-Path $noticeStage "VERSION.txt"),
    (Join-Path $noticeStage $readme.Name),
    (Join-Path $noticeStage $license.Name)
)
foreach ($artifact in $requiredArtifacts) {
    Require-File $artifact "staged FFmpeg artifact"
}

Write-Host "Staged FFmpeg binaries in $binaryStage"
Write-Host "Staged FFmpeg notices in $noticeStage"
