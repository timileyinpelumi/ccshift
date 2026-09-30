# Installs the latest ccshift release for Windows.
#
#   irm https://www.timileyin.dev/ccshift/install.ps1 | iex
#
# $env:CCSHIFT_INSTALL_DIR  where ccshift.exe goes (default: %LOCALAPPDATA%\Programs\ccshift)
# $env:CCSHIFT_VERSION      a release tag such as v0.2.0 (default: the latest release)
$ErrorActionPreference = 'Stop'

function Install-Ccshift {
    $repo = 'timileyinpelumi/ccshift'
    $dir = if ($env:CCSHIFT_INSTALL_DIR) { $env:CCSHIFT_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\ccshift' }
    $arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
    $base = if ($env:CCSHIFT_VERSION) { "https://github.com/$repo/releases/download/$($env:CCSHIFT_VERSION)" } else { "https://github.com/$repo/releases/latest/download" }
    $asset = "ccshift_windows_$arch.zip"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("ccshift-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $asset"
        Invoke-WebRequest -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset) -UseBasicParsing
        Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

        $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match "\s$([regex]::Escape($asset))$" }
        if (-not $line) { throw "checksums.txt has no entry for $asset" }
        $want = ($line -split '\s+')[0]
        $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLower()
        if ($want -ne $got) { throw 'The download does not match its checksum. Nothing was installed.' }

        Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath $tmp -Force
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Copy-Item (Join-Path $tmp 'ccshift.exe') (Join-Path $dir 'ccshift.exe') -Force
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }

    $version = & (Join-Path $dir 'ccshift.exe') version
    Write-Host "Installed ccshift $version to $dir"

    $path = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($path -split ';') -notcontains $dir) {
        [Environment]::SetEnvironmentVariable('Path', "$path;$dir", 'User')
        Write-Host "Added $dir to your PATH. Open a new terminal for it to take effect."
    }
    Write-Host 'Next: ccshift init'
}

Install-Ccshift
