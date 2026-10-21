# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

[CmdletBinding()]
param(
    [string[]] $VhdxPath,
    [switch] $SkipShutdown,
    [int] $ShutdownTimeoutSeconds = 60
)

$ErrorActionPreference = 'Stop'

function Test-Elevated {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-DefaultVhdxPath {
    $roots = @(
        'D:\Hyper-V\Virtual Hard Disks',
        'D:\Hyper-V\Windows 2025\Virtual Hard Disks',
        'D:\WSL',
        (Join-Path $env:LOCALAPPDATA 'Docker\wsl'),
        (Join-Path $env:LOCALAPPDATA 'Packages')
    ) | Where-Object { $_ -and (Test-Path $_) }

    if (-not $roots) { return @() }

    return Get-ChildItem -Path $roots -Filter '*.vhdx' -Recurse -File -ErrorAction SilentlyContinue |
        Select-Object -ExpandProperty FullName
}

function Wait-WslStopped {
    param([int] $TimeoutSeconds)

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $output = (wsl.exe -l -v 2>&1) -join "`n"
        $clean = $output -replace "`0", ''
        if ($clean -notmatch 'Running') { return $true }
        Start-Sleep -Seconds 2
    }
    return $false
}

function Invoke-CompactVdisk {
    param([string] $Path)

    $script = @(
        "select vdisk file=`"$Path`"",
        'attach vdisk readonly',
        'compact vdisk',
        'detach vdisk',
        'exit'
    ) -join "`r`n"

    $scriptFile = Join-Path ([IO.Path]::GetTempPath()) ("compact-vdisk-{0}.txt" -f ([guid]::NewGuid().ToString('N')))
    Set-Content -Path $scriptFile -Value $script -Encoding ascii

    try {
        $output = & diskpart.exe /s $scriptFile 2>&1
        return [pscustomobject]@{
            ExitCode = $LASTEXITCODE
            Output   = ($output | Out-String).Trim()
        }
    } finally {
        Remove-Item -Path $scriptFile -Force -ErrorAction SilentlyContinue
    }
}

# if (-not (Test-Elevated)) {
#     throw 'This script must be run from an elevated PowerShell session (diskpart requires Administrator).'
# }

if (-not $VhdxPath -or $VhdxPath.Count -eq 0) {
    $VhdxPath = Get-DefaultVhdxPath
    if (-not $VhdxPath -or $VhdxPath.Count -eq 0) {
        throw 'No .vhdx files found. Pass -VhdxPath "D:\WSL\DockerDesktopWSL\disk\docker_data.vhdx".'
    }
}

$targets = @()
foreach ($path in $VhdxPath) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        Write-Warning "Skipping missing file: $path"
        continue
    }
    $targets += (Resolve-Path -LiteralPath $path).ProviderPath
}

if ($targets.Count -eq 0) {
    throw 'No existing .vhdx files to compact.'
}

Write-Host "Targets:"
$targets | ForEach-Object { Write-Host "  $_" }

if (-not $SkipShutdown) {
    Write-Host 'Shutting down WSL...'
    wsl.exe --shutdown | Out-Null
    if (-not (Wait-WslStopped -TimeoutSeconds $ShutdownTimeoutSeconds)) {
        throw "WSL still reports a running distribution after $ShutdownTimeoutSeconds seconds. Stop Docker Desktop and retry."
    }
    Start-Sleep -Seconds 5
    Write-Host 'WSL stopped.'
}

$results = @()
foreach ($target in $targets) {
    $before = (Get-Item -LiteralPath $target).Length
    Write-Host ("Compacting {0} ({1:N2} GB)..." -f $target, ($before / 1GB))

    $result = Invoke-CompactVdisk -Path $target
    if ($result.ExitCode -ne 0) {
        Write-Warning "diskpart failed for $target (exit $($result.ExitCode)):`n$($result.Output)"
        continue
    }

    $after = (Get-Item -LiteralPath $target).Length
    $results += [pscustomobject]@{
        Path      = $target
        BeforeGB  = [math]::Round($before / 1GB, 2)
        AfterGB   = [math]::Round($after / 1GB, 2)
        SavedGB   = [math]::Round(($before - $after) / 1GB, 2)
    }
}

if ($results.Count -gt 0) {
    $results | Format-Table -AutoSize
    Write-Host ("Total reclaimed: {0:N2} GB" -f (($results | Measure-Object -Property SavedGB -Sum).Sum))
} else {
    Write-Warning 'Nothing was compacted.'
}
