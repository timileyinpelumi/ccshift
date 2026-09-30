# Installs the latest ccshift release for Windows.
#
#   irm https://www.timileyin.dev/ccshift/install.ps1 | iex
#
# $env:CCSHIFT_INSTALL_DIR  where ccshift.exe goes (default: %LOCALAPPDATA%\Programs\ccshift)
# $env:CCSHIFT_VERSION      a release tag such as v0.2.0 (default: the latest release)
# $env:CCSHIFT_NO_SETUP     set to skip the question about running ccshift init
$ErrorActionPreference = 'Stop'

function Install-Ccshift {
    $repo = 'timileyinpelumi/ccshift'
    $dir = if ($env:CCSHIFT_INSTALL_DIR) { $env:CCSHIFT_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\ccshift' }
    $arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
    $base = if ($env:CCSHIFT_VERSION) { "https://github.com/$repo/releases/download/$($env:CCSHIFT_VERSION)" } else { "https://github.com/$repo/releases/latest/download" }
    $asset = "ccshift_windows_$arch.zip"
    $tick = [char]0x2713

    function Step($text) { Write-Host '> ' -ForegroundColor Cyan -NoNewline; Write-Host $text -NoNewline }
    function Done($text) { Write-Host "  $tick $text" -ForegroundColor Green }

    Write-Host ''
    Write-Host '                  __    _ ______' -ForegroundColor Yellow
    Write-Host '  _______________/ /_  (_) __/ /_' -ForegroundColor Yellow
    Write-Host ' / ___/ ___/ ___/ __ \/ / /_/ __/' -ForegroundColor Yellow
    Write-Host '/ /__/ /__(__  ) / / / / __/ /_' -ForegroundColor Yellow
    Write-Host '\___/\___/____/_/ /_/_/_/  \__/' -ForegroundColor Yellow
    Write-Host ''
    Write-Host 'Keep your Claude Code sessions across restarts.' -ForegroundColor DarkGray
    Write-Host ''

    Step 'Checking this machine'
    Done "windows $arch"

    $exe = Join-Path $dir 'ccshift.exe'
    if (Test-Path $exe) {
        $current = & $exe version
        $tag = $env:CCSHIFT_VERSION
        if (-not $tag) {
            try {
                $r = Invoke-WebRequest -Uri "https://github.com/$repo/releases/latest" -Method Head -MaximumRedirection 0 -UseBasicParsing -ErrorAction SilentlyContinue
            } catch { $r = $_.Exception.Response }
            if ($r -and $r.Headers.Location) { $tag = ($r.Headers.Location -split '/')[-1] }
        }
        Write-Host ''
        Write-Host "ccshift $current is already installed at $exe."
        if ([Environment]::UserInteractive -and -not $env:CI) {
            if ($tag -and "v$current" -eq $tag) {
                $answer = Read-Host 'It is the latest version. Reinstall it anyway? [y/N]'
                if ($answer -notmatch '^[yY]') { Write-Host 'Nothing changed.'; return }
            } else {
                $answer = Read-Host "Replace it with $(if ($tag) { $tag } else { 'the latest release' })? [Y/n]"
                if ($answer -match '^[nN]') { Write-Host 'Nothing changed.'; return }
            }
        }
        Write-Host ''
    }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("ccshift-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Step "Downloading $asset"
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset) -UseBasicParsing
        Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing
        Done ('{0:N1} MB' -f ((Get-Item (Join-Path $tmp $asset)).Length / 1MB))

        Step 'Checking the checksum'
        $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match "\s$([regex]::Escape($asset))$" }
        if (-not $line) { throw "checksums.txt has no entry for $asset" }
        $want = ($line -split '\s+')[0]
        $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLower()
        if ($want -ne $got) { throw 'The download does not match its checksum. Nothing was installed.' }
        Done 'sha256 matches'

        Step "Installing to $dir"
        Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath $tmp -Force
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Copy-Item (Join-Path $tmp 'ccshift.exe') (Join-Path $dir 'ccshift.exe') -Force
        $version = & (Join-Path $dir 'ccshift.exe') version
        Done "ccshift $version"
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }

    $path = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($path -split ';') -notcontains $dir) {
        Step 'Adding it to your PATH'
        [Environment]::SetEnvironmentVariable('Path', "$path;$dir", 'User')
        $env:Path = "$env:Path;$dir"
        Done 'open a new terminal for other windows to see it'
    }

    Write-Host ''
    Write-Host 'ccshift is installed.' -ForegroundColor Green

    $setUp = $false
    if (-not $env:CCSHIFT_NO_SETUP -and [Environment]::UserInteractive) {
        Write-Host ''
        $answer = Read-Host 'Turn on autosave now? It adds three hooks to Claude Code and backs up your settings. [Y/n]'
        if ($answer -notmatch '^[nN]') {
            Write-Host ''
            & (Join-Path $dir 'ccshift.exe') init
            $setUp = $LASTEXITCODE -eq 0
        }
    }

    Write-Host ''
    Write-Host 'Next'
    if (-not $setUp) { Write-Host '  ccshift init       ' -ForegroundColor Cyan -NoNewline; Write-Host 'turn on autosave' }
    Write-Host '  ccshift doctor     ' -ForegroundColor Cyan -NoNewline; Write-Host 'check the setup'
    Write-Host '  ccshift restore    ' -ForegroundColor Cyan -NoNewline; Write-Host 'bring your sessions back after a restart'
    Write-Host ''
    Write-Host 'Docs: https://www.timileyin.dev/ccshift' -ForegroundColor DarkGray
    Write-Host 'Support ccshift: https://paystack.shop/pay/ne1sknbf3o' -ForegroundColor DarkGray
}

Install-Ccshift
