$ErrorActionPreference = "Stop"

$repository = if ($env:PROSECHECK_REPOSITORY) { $env:PROSECHECK_REPOSITORY } else { "DheerG/prosecheck" }
$userDirectory = [Environment]::GetFolderPath("UserProfile")
$installDirectory = if ($env:PROSECHECK_INSTALL_DIR) { $env:PROSECHECK_INSTALL_DIR } else { Join-Path $userDirectory ".local\bin" }

switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
    "X64" { $architecture = "amd64" }
    "Arm64" { $architecture = "arm64" }
    default { throw "Cannot install prosecheck: this processor is not supported." }
}

if ($env:PROSECHECK_VERSION) {
    $tag = $env:PROSECHECK_VERSION
    if (-not $tag.StartsWith("v")) { $tag = "v$tag" }
} else {
    $headers = @{
        "Accept" = "application/vnd.github+json"
        "X-GitHub-Api-Version" = "2022-11-28"
    }
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repository/releases/latest" -Headers $headers
    $tag = $release.tag_name
}

if (-not $tag) { throw "Cannot install prosecheck: the latest release tag was empty." }

$asset = "prosecheck_windows_$architecture.zip"
$downloadRoot = "https://github.com/$repository/releases/download/$tag"
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("prosecheck-install-" + [guid]::NewGuid().ToString("N"))

New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null

try {
    $archivePath = Join-Path $temporaryDirectory $asset
    $checksumPath = Join-Path $temporaryDirectory "checksums.txt"

    Write-Host "Downloading prosecheck $tag for windows/$architecture..."
    Invoke-WebRequest -Uri "$downloadRoot/$asset" -OutFile $archivePath -UseBasicParsing
    Invoke-WebRequest -Uri "$downloadRoot/checksums.txt" -OutFile $checksumPath -UseBasicParsing

    $checksumLine = Get-Content $checksumPath | Where-Object { $_ -match "^[a-fA-F0-9]{64}\s+$([regex]::Escape($asset))$" } | Select-Object -First 1
    if (-not $checksumLine) { throw "The release does not contain a checksum for $asset." }

    $expected = ($checksumLine -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "The release archive checksum does not match." }

    $extractDirectory = Join-Path $temporaryDirectory "archive"
    Expand-Archive -Path $archivePath -DestinationPath $extractDirectory
    New-Item -ItemType Directory -Force -Path $installDirectory | Out-Null
    Copy-Item -Path (Join-Path $extractDirectory "prosecheck.exe") -Destination (Join-Path $installDirectory "prosecheck.exe") -Force

    Write-Host "Installed prosecheck $tag at $installDirectory\prosecheck.exe."
    $pathEntries = $env:PATH -split ";"
    if ($pathEntries -notcontains $installDirectory) {
        Write-Host "Add $installDirectory to PATH, then run: prosecheck init"
    }
} catch {
    throw "Cannot install prosecheck: $($_.Exception.Message)"
} finally {
    Remove-Item -Path $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
