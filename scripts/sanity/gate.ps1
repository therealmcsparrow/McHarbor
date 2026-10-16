# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
[CmdletBinding()]
param(
    [string[]]$Paths,
    [string]$Since,
    [switch]$GitStatus
)

$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$node = Get-Command node -ErrorAction SilentlyContinue
if (-not $node) {
    [Console]::Error.WriteLine('gate.ps1: node was not found on PATH')
    exit 2
}

$argList = @((Join-Path $repo 'scripts\hooks\gate-touched.mjs'), '--cli')
if ($Paths) { $argList += '--paths'; $argList += (($Paths | ForEach-Object { $_ -split ',' } | Where-Object { $_ }) -join ',') }
if ($Since) { $argList += '--since'; $argList += $Since }
if ($GitStatus) { $argList += '--git-status' }

& $node.Source @argList
exit $LASTEXITCODE
