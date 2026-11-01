# Installs the latest agentsync release from GitHub.
#
# Usage (Windows PowerShell 5.1 and PowerShell 7):
#   irm https://raw.githubusercontent.com/x0c/agentsync/main/install.ps1 | iex
#
# Environment variables:
#   AGENTSYNC_INSTALL_DIR       installation directory (default: %LOCALAPPDATA%\Programs\agentsync\bin)
#   AGENTSYNC_VERSION           version to install, e.g. 1.4.0 or v1.4.0 (default: latest release)
#   AGENTSYNC_REPOSITORY        GitHub owner/name (default: x0c/agentsync)
#   AGENTSYNC_SKIP_PATH_UPDATE  set to 1 to skip adding the directory to the user PATH
#
# The installer verifies the downloaded archive against the release
# checksums.txt (SHA-256) before installing. No secrets, no telemetry.

[CmdletBinding()]
param(
    [string]$Version,
    [switch]$SkipPathUpdate
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Write-Step {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '',
        Justification = 'Installer progress output must reach the console on Windows PowerShell 5.1.')]
    param([string]$Message)
    Write-Host "agentsync: $Message"
}

function Get-RequestHeader {
    $headers = @{
        Accept = "application/vnd.github+json"
        "User-Agent" = "agentsync-installer"
        "X-GitHub-Api-Version" = "2022-11-28"
    }
    $token = if ($env:GH_TOKEN) { $env:GH_TOKEN } elseif ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN } else { $null }
    if ($token) {
        $headers.Authorization = "Bearer $token"
    }
    return $headers
}

function Add-ToUserPath {
    param([string]$Directory)

    $trimCharacters = [char[]]"\/"
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @()
    if ($userPath) {
        $entries = @($userPath -split ";" | Where-Object { $_ })
    }
    $normalizedDirectory = $Directory.TrimEnd($trimCharacters)
    $alreadyPresent = $entries | Where-Object {
        $_.Trim().TrimEnd($trimCharacters).Equals($normalizedDirectory, [StringComparison]::OrdinalIgnoreCase)
    }
    if (-not $alreadyPresent) {
        $newPath = (@($entries) + $Directory) -join ";"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Step "Added $Directory to your user PATH. Open a new terminal to use it."
    }
    $processEntries = @($env:Path -split ";" | Where-Object { $_ })
    $processPresent = $processEntries | Where-Object {
        $_.Trim().TrimEnd($trimCharacters).Equals($normalizedDirectory, [StringComparison]::OrdinalIgnoreCase)
    }
    if (-not $processPresent) {
        $env:Path = (@($processEntries) + $Directory) -join ";"
    }
}

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

    if (-not $Version -and $env:AGENTSYNC_VERSION) {
        $Version = $env:AGENTSYNC_VERSION
    }
    if ($Version -and $Version -notmatch '^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$') {
        throw "Invalid version '$Version'. Expected a value such as 1.4.0 or v1.4.0."
    }

    $repository = if ($env:AGENTSYNC_REPOSITORY) { $env:AGENTSYNC_REPOSITORY } else { "x0c/agentsync" }
    if ($repository -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') {
        throw "Invalid repository '$repository'. Expected owner/name."
    }
    $headers = Get-RequestHeader
    $apiBase = "https://api.github.com/repos/$repository/releases"

    if ($Version) {
        $tag = if ($Version.StartsWith("v")) { $Version } else { "v$Version" }
        $encodedTag = [Uri]::EscapeDataString($tag)
        Write-Step "Resolving release $tag..."
        $release = Invoke-RestMethod -Uri "$apiBase/tags/$encodedTag" -Headers $headers
    }
    else {
        Write-Step "Resolving the latest release..."
        $release = Invoke-RestMethod -Uri "$apiBase/latest" -Headers $headers
        $tag = [string]$release.tag_name
    }

    if ($release.draft) {
        throw "Release $tag is still a draft and cannot be installed."
    }

    $machineArchitecture = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrEmpty($machineArchitecture)) {
        $machineArchitecture = $env:PROCESSOR_ARCHITECTURE
    }
    if ([string]::IsNullOrEmpty($machineArchitecture)) {
        throw "Could not determine the machine architecture. agentsync provides Windows builds for amd64 and arm64."
    }
    switch ($machineArchitecture.ToUpperInvariant()) {
        "AMD64" { $goarch = "amd64" }
        "ARM64" { $goarch = "arm64" }
        default { throw "Unsupported architecture '$machineArchitecture'. agentsync provides Windows builds for amd64 and arm64." }
    }

    $archiveName = "agentsync_windows_$goarch.zip"
    $archiveAsset = @($release.assets | Where-Object { $_.name -eq $archiveName }) | Select-Object -First 1
    $checksumAsset = @($release.assets | Where-Object { $_.name -eq "checksums.txt" }) | Select-Object -First 1
    if (-not $archiveAsset) {
        throw "Release $tag does not publish $archiveName. A Windows $goarch build is not available in this release."
    }
    if (-not $checksumAsset) {
        throw "Release $tag does not publish checksums.txt and cannot be verified."
    }

    $installDirectory = if ($env:AGENTSYNC_INSTALL_DIR) {
        [IO.Path]::GetFullPath($env:AGENTSYNC_INSTALL_DIR)
    }
    else {
        if (-not $env:LOCALAPPDATA) {
            throw "LOCALAPPDATA is unavailable. Set AGENTSYNC_INSTALL_DIR to choose an installation directory."
        }
        Join-Path $env:LOCALAPPDATA "Programs\agentsync\bin"
    }

    $temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("agentsync-install-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
    try {
        $archivePath = Join-Path $temporaryDirectory $archiveName
        $checksumPath = Join-Path $temporaryDirectory "checksums.txt"
        Write-Step "Downloading $tag for Windows ($goarch)..."
        Invoke-WebRequest -Uri $archiveAsset.browser_download_url -Headers $headers -OutFile $archivePath -UseBasicParsing
        Invoke-WebRequest -Uri $checksumAsset.browser_download_url -Headers $headers -OutFile $checksumPath -UseBasicParsing

        $expectedHash = $null
        foreach ($line in (Get-Content -LiteralPath $checksumPath)) {
            $match = [Regex]::Match($line, '(?i)^\s*([0-9a-f]{64})\s+\*?' + [Regex]::Escape($archiveName) + '\s*$')
            if ($match.Success) {
                $expectedHash = $match.Groups[1].Value.ToLowerInvariant()
                break
            }
        }
        if (-not $expectedHash) {
            throw "The release checksum file does not contain an entry for $archiveName."
        }
        $actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualHash -ne $expectedHash) {
            throw "SHA256 verification failed. The downloaded file was not installed."
        }
        Write-Step "SHA256 verification passed."

        $extractDirectory = Join-Path $temporaryDirectory "extracted"
        Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDirectory
        $executables = @(Get-ChildItem -LiteralPath $extractDirectory -Filter "agentsync.exe" -File -Recurse)
        if ($executables.Count -ne 1) {
            throw "The release archive does not contain exactly one agentsync.exe."
        }

        New-Item -ItemType Directory -Force -Path $installDirectory | Out-Null
        $destination = Join-Path $installDirectory "agentsync.exe"
        $staging = Join-Path $installDirectory ("agentsync.exe.new-" + [Guid]::NewGuid().ToString("N"))
        try {
            Copy-Item -LiteralPath $executables[0].FullName -Destination $staging
            Move-Item -LiteralPath $staging -Destination $destination -Force
        }
        finally {
            if (Test-Path -LiteralPath $staging) {
                Remove-Item -LiteralPath $staging -Force
            }
        }

        $skipPath = $SkipPathUpdate -or $env:AGENTSYNC_SKIP_PATH_UPDATE -eq "1"
        if (-not $skipPath) {
            Add-ToUserPath -Directory $installDirectory
        }
        Write-Step "Installed $tag to $destination"
        Write-Step "Run 'agentsync --version' to verify the installation."
    }
    finally {
        if ($temporaryDirectory -and (Test-Path -LiteralPath $temporaryDirectory)) {
            Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force
        }
    }
}
catch {
    Write-Error "agentsync installation failed: $($_.Exception.Message)"
    exit 1
}
