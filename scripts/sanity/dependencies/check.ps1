# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Dependency freshness sanity check. Mirrors .instructions/sanity/dependencies.md.
.DESCRIPTION
  Informational by default: lists direct npm and Go dependencies with newer
  versions, classified patch/minor/major, plus local import counts. Fails only
  on confirmed vulnerabilities (npm audit high/critical, govulncheck findings).
  The "What changed" column requires upstream release notes and is left for
  the reviewer. Needs network; pass -SkipNetwork to produce a skipped report.
.EXAMPLE
  .\scripts\sanity\dependencies\check.ps1
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [switch]$SkipNetwork,
  [int]$CommandTimeoutSec = 300
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$report = New-SanityReport -Name 'dependencies' -Title 'Dependency Freshness Sanity' -Scope 'frontend npm, backend Go, agent Go' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$frontendDir = Join-Path $Root 'src\frontend'
$npm = Resolve-SanityTool 'npm'
$go = Resolve-SanityTool 'go'
$govuln = Resolve-SanityTool 'govulncheck'
$headers = @('Dependency', 'Ecosystem', 'Current', 'Target', 'Update type', 'What changed', 'Code impact', 'Risk', 'Recommendation')

function Get-SanitySemver([string]$v) {
  $m = [regex]::Match("$v", '(\d+)\.(\d+)\.(\d+)')
  if ($m.Success) { return @([int]$m.Groups[1].Value, [int]$m.Groups[2].Value, [int]$m.Groups[3].Value) }
  return $null
}
function Get-SanityUpdateType([string]$cur, [string]$target) {
  if ("$target" -match '-\d{14}-[0-9a-f]{12}') { return 'pseudo-version' }
  $a = Get-SanitySemver $cur; $b = Get-SanitySemver $target
  if (-not $a -or -not $b) { return 'unknown' }
  if ($b[0] -ne $a[0]) { return 'major' }
  if ($b[1] -ne $a[1]) { return 'minor' }
  if ($b[2] -ne $a[2]) { return 'patch' }
  return 'none'
}
function Get-SanityRisk([string]$type) { switch ($type) { 'major' { 'medium' } 'pseudo-version' { 'low' } default { 'low' } } }
function Get-SanityRecommendation([string]$type) { switch ($type) { 'major' { 'plan migration' } 'minor' { 'update now' } 'patch' { 'update now' } default { 'monitor' } } }

# A dev tool is "used" when a config file names it or a package.json script runs its
# binary, even though no source file imports it.
function Find-SanityNpmConfigUse([string]$Name, $PkgJson, [string]$FrontendDir) {
  $bare = ($Name -replace '^@[^/]+/', '')
  $needles = @($Name, $bare) | Select-Object -Unique
  $scripts = Get-SanityJsonValue $PkgJson 'scripts'
  if ($scripts -is [System.Collections.IDictionary]) {
    foreach ($k in $scripts.Keys) {
      foreach ($n in $needles) { if ("$($scripts[$k])" -match ('(^|[\s;&|"/])' + [regex]::Escape($n) + '([\s"]|$)')) { return "package.json script '$k'" } }
    }
  }
  if ($Name -match '^@vitest/coverage-(\w+)$') {
    $prov = $Matches[1]
    if ($scripts -is [System.Collections.IDictionary]) { foreach ($k in $scripts.Keys) { if ("$($scripts[$k])" -match '--coverage') { return "package.json script '$k' (--coverage)" } } }
    foreach ($c in @(Get-ChildItem -LiteralPath $FrontendDir -File -ErrorAction SilentlyContinue | Where-Object { $_.Name -match '^(vite|vitest)\.config\.' })) {
      if ((Get-SanityText $c.FullName) -match ("provider:\s*['`"]" + $prov + "['`"]")) { return "$($c.Name) coverage.provider" }
    }
  }
  $cfgs = @(Get-ChildItem -LiteralPath $FrontendDir -File -ErrorAction SilentlyContinue | Where-Object { $_.Name -match '^(.+\.config\.(js|ts|mjs|cjs|mts|cts)|knip\.jsonc?|\.knip\.jsonc?|\.eslintrc(\.\w+)?|vitest\.workspace\.\w+)$' })
  foreach ($c in $cfgs) {
    $txt = Get-SanityText $c.FullName
    foreach ($n in $needles) { if ($txt -match ('(^|[^\w@/.-])' + [regex]::Escape($n) + '([^\w-]|$)')) { return $c.Name } }
  }
  return $null
}

# 1. Frontend outdated packages
$rowsNpm = @()
if ($SkipNetwork) { Add-SanityCheck -Report $report -Id 1 -Title 'Frontend outdated packages (direct deps)' -Skipped -SkipReason '-SkipNetwork' }
elseif (-not $npm) { Add-SanityCheck -Report $report -Id 1 -Title 'Frontend outdated packages (direct deps)' -Skipped -SkipReason 'npm not found on PATH' }
else {
  $pkgJson = $null
  try { $pkgJson = ConvertFrom-SanityJson (Get-SanityText (Join-Path $frontendDir 'package.json')) } catch { }
  $direct = @{}
  foreach ($section in @('dependencies', 'devDependencies')) {
    $d = Get-SanityJsonValue $pkgJson $section
    if ($d -is [System.Collections.IDictionary]) { foreach ($k in $d.Keys) { $direct[$k] = $section } }
  }
  $r = Invoke-SanityCommand -FilePath $npm -Arguments 'outdated --json --loglevel error' -WorkingDirectory $frontendDir -TimeoutSec $CommandTimeoutSec
  if ($r.TimedOut) { Add-SanityCheck -Report $report -Id 1 -Title 'Frontend outdated packages (direct deps)' -Skipped -SkipReason 'npm outdated timed out' }
  else {
    $data = $null
    if (-not [string]::IsNullOrWhiteSpace($r.StdOut)) { try { $data = ConvertFrom-SanityJson $r.StdOut } catch { $data = $null } }
    if ($data -is [System.Collections.IDictionary]) {
      foreach ($name in ($data.Keys | Sort-Object)) {
        if (-not $direct.ContainsKey($name)) { continue }
        $e = $data[$name]
        $cur = "$(Get-SanityJsonValue $e 'current')"; $wanted = "$(Get-SanityJsonValue $e 'wanted')"; $latest = "$(Get-SanityJsonValue $e 'latest')"
        $target = if ($wanted -and $wanted -ne $cur) { $wanted } else { $latest }
        $type = Get-SanityUpdateType $cur $target
        if ($type -eq 'none') { continue }
        $uses = @(Find-Pattern -Root $Root -Paths @('src/frontend') -Include @('*.ts', '*.tsx', '*.js', '*.mjs', '*.cjs', '*.css') -Pattern ("from ['`"]" + [regex]::Escape($name) + "(/|['`"])|require\(['`"]" + [regex]::Escape($name) + "|import\(\s*['`"]" + [regex]::Escape($name) + "|@import ['`"]" + [regex]::Escape($name)))
        $cfgUse = $null
        if ($uses.Count -eq 0) { $cfgUse = Find-SanityNpmConfigUse -Name $name -PkgJson $pkgJson -FrontendDir $frontendDir }
        $impact = if ($uses.Count -gt 0) { "$($uses.Count) import sites, e.g. $($uses[0].File)" } elseif ($cfgUse) { "used by tooling: $cfgUse" } else { "no import found in src/frontend ($($direct[$name])) - verify usage or remove" }
        $rowsNpm += ,@($name, 'npm', $cur, ("{0} (latest {1})" -f $target, $latest), $type, 'review upstream changelog', $impact, (Get-SanityRisk $type), (Get-SanityRecommendation $type))
      }
    }
    Add-SanityCheck -Report $report -Id 1 -Title 'Frontend outdated packages (direct deps)' -Hits @($rowsNpm | ForEach-Object { "{0}: {1} -> {2} ({3})" -f $_[0], $_[2], $_[3], $_[4] }) -Mode info -Severity low `
      -Table @{ Headers = $headers; Rows = $rowsNpm } -Note 'Target = npm "wanted" when it differs from current, else "latest". Fill in "What changed" from the upstream changelog before recommending an update.' -Command 'cd src/frontend; npm outdated --json'
  }
}

# 2/3. Go module updates
# The host go is pinned to an older toolchain than go.mod asks for, so go list
# needs GOTOOLCHAIN=auto and the repo-level go.work out of the way.
$goEnvPrev = @{ GOTOOLCHAIN = $env:GOTOOLCHAIN; GOWORK = $env:GOWORK }
$env:GOTOOLCHAIN = 'auto'; $env:GOWORK = 'off'
function Read-SanityGoModList([string]$out) {
  $objs = @(); $buf = New-Object System.Text.StringBuilder; $depth = 0
  foreach ($line in ($out -split "`r?`n")) {
    $opens = ([regex]::Matches($line, '\{')).Count; $closes = ([regex]::Matches($line, '\}')).Count
    [void]$buf.AppendLine($line)
    $depth += $opens - $closes
    if ($depth -eq 0 -and $buf.Length -gt 2) { $txt = $buf.ToString().Trim(); if ($txt) { try { $objs += ConvertFrom-SanityJson $txt } catch { } }; [void]$buf.Clear() }
  }
  return $objs
}
$goIndex = 2
foreach ($modDir in @('src/backend', 'src/agent')) {
  $eco = if ($modDir -eq 'src/backend') { 'backend-go' } else { 'agent-go' }
  $title = "Go module updates ($modDir, direct modules)"
  if ($SkipNetwork) { Add-SanityCheck -Report $report -Id $goIndex -Title $title -Skipped -SkipReason '-SkipNetwork'; $goIndex++; continue }
  if (-not $go) { Add-SanityCheck -Report $report -Id $goIndex -Title $title -Skipped -SkipReason 'go not found on PATH'; $goIndex++; continue }
  $r = Invoke-SanityCommand -FilePath $go -Arguments 'list -m -u -json all' -WorkingDirectory (Join-Path $Root $modDir) -TimeoutSec $CommandTimeoutSec
  if ($r.Failed -or [string]::IsNullOrWhiteSpace($r.StdOut)) { Add-SanityCheck -Report $report -Id $goIndex -Title $title -Skipped -SkipReason ("go list failed: " + (($r.StdErr -split "`n") | Select-Object -First 1)); $goIndex++; continue }
  $rows = @(); $indirectUpdates = 0
  foreach ($m in (Read-SanityGoModList $r.StdOut)) {
    if (Get-SanityJsonValue $m 'Main') { continue }
    $upd = Get-SanityJsonValue $m 'Update'
    if ($null -eq $upd) { continue }
    if (Get-SanityJsonValue $m 'Indirect') { $indirectUpdates++; continue }
    $path = "$(Get-SanityJsonValue $m 'Path')"; $cur = "$(Get-SanityJsonValue $m 'Version')"; $target = "$(Get-SanityJsonValue $upd 'Version')"
    $type = Get-SanityUpdateType $cur $target
    $uses = @(Find-Pattern -Root $Root -Paths @($modDir) -Include @('*.go') -Pattern ('"' + [regex]::Escape($path) + '(/|")'))
    $impact = if ($uses.Count -gt 0) { "$($uses.Count) import sites, e.g. $($uses[0].File)" } else { 'no direct import found (build tag, tool, or replace directive) - verify' }
    if ($type -eq 'major' -and $path -match '/v\d+$') { $impact += '; semantic import versioning: import path changes' }
    $rows += ,@($path, $eco, $cur, $target, $type, 'review upstream changelog', $impact, (Get-SanityRisk $type), (Get-SanityRecommendation $type))
  }
  Add-SanityCheck -Report $report -Id $goIndex -Title $title -Hits @($rows | ForEach-Object { "{0}: {1} -> {2} ({3})" -f $_[0], $_[2], $_[3], $_[4] }) -Mode info -Severity low `
    -Table @{ Headers = $headers; Rows = $rows } -Note ("{0} indirect modules also have updates (not listed). WebSocket, env parsing, Docker proxy and toolchain modules deserve extra attention for the agent." -f $indirectUpdates) -Command "cd $modDir; go list -m -u -json all"
  $goIndex++
}

foreach ($k in $goEnvPrev.Keys) { if ($null -eq $goEnvPrev[$k]) { Remove-Item "Env:$k" -ErrorAction SilentlyContinue } else { Set-Item "Env:$k" $goEnvPrev[$k] } }

# 4. Security and compatibility context
if ($SkipNetwork) { Add-SanityCheck -Report $report -Id '4a' -Title 'npm audit --omit=dev (high/critical)' -Skipped -SkipReason '-SkipNetwork' }
elseif (-not $npm) { Add-SanityCheck -Report $report -Id '4a' -Title 'npm audit --omit=dev (high/critical)' -Skipped -SkipReason 'npm not found on PATH' }
else {
  $r = Invoke-SanityCommand -FilePath $npm -Arguments 'audit --omit=dev --json --loglevel error' -WorkingDirectory $frontendDir -TimeoutSec $CommandTimeoutSec
  $vulnHits = @(); $note = ''
  if ($r.TimedOut) { Add-SanityCheck -Report $report -Id '4a' -Title 'npm audit --omit=dev (high/critical)' -Skipped -SkipReason 'npm audit timed out' }
  else {
    $data = $null
    if (-not [string]::IsNullOrWhiteSpace($r.StdOut)) { try { $data = ConvertFrom-SanityJson $r.StdOut } catch { $data = $null } }
    $vulns = Get-SanityJsonValue $data 'vulnerabilities'
    $meta = Get-SanityJsonValue (Get-SanityJsonValue $data 'metadata') 'vulnerabilities'
    if ($meta) { $note = "npm audit totals: critical={0} high={1} moderate={2} low={3}" -f (Get-SanityJsonValue $meta 'critical'), (Get-SanityJsonValue $meta 'high'), (Get-SanityJsonValue $meta 'moderate'), (Get-SanityJsonValue $meta 'low') }
    if ($vulns -is [System.Collections.IDictionary]) {
      foreach ($name in $vulns.Keys) {
        $v = $vulns[$name]; $sev = "$(Get-SanityJsonValue $v 'severity')"
        if ($sev -in @('high', 'critical')) { $vulnHits += New-SanityHit -File 'src/frontend/package-lock.json' -Line 0 -Text ("{0}: {1} (range {2}, fixAvailable={3})" -f $name, $sev, (Get-SanityJsonValue $v 'range'), (Get-SanityJsonValue $v 'fixAvailable')) }
      }
    } elseif ($null -eq $data) { $note = 'npm audit returned no JSON (registry unreachable?); treat as skipped: ' + (($r.StdErr -split "`n") | Select-Object -First 1) }
    Add-SanityCheck -Report $report -Id '4a' -Title 'npm audit --omit=dev (high/critical)' -Hits $vulnHits -Mode strict -Severity high -Note $note -Remediation 'Apply npm audit fix or upgrade the parent dependency; document accepted risks.' -Command 'cd src/frontend; npm audit --omit=dev'
  }
}
foreach ($modDir in @('src/backend', 'src/agent')) {
  $id = if ($modDir -eq 'src/backend') { '4b' } else { '4c' }
  $title = "govulncheck ./... ($modDir)"
  if ($SkipNetwork) { Add-SanityCheck -Report $report -Id $id -Title $title -Skipped -SkipReason '-SkipNetwork'; continue }
  if (-not $govuln) { Add-SanityCheck -Report $report -Id $id -Title $title -Skipped -SkipReason 'govulncheck not installed (go install golang.org/x/vuln/cmd/govulncheck@latest)'; continue }
  $prevGoWork = $env:GOWORK
  $prevGoToolchain = $env:GOTOOLCHAIN
  $env:GOWORK = 'off'
  $env:GOTOOLCHAIN = 'auto'
  try { $r = Invoke-SanityCommand -FilePath $govuln -Arguments './...' -WorkingDirectory (Join-Path $Root $modDir) -TimeoutSec ($CommandTimeoutSec * 2) }
  finally {
    if ($null -eq $prevGoWork) { Remove-Item Env:GOWORK -ErrorAction SilentlyContinue } else { $env:GOWORK = $prevGoWork }
    if ($null -eq $prevGoToolchain) { Remove-Item Env:GOTOOLCHAIN -ErrorAction SilentlyContinue } else { $env:GOTOOLCHAIN = $prevGoToolchain }
  }
  if ($r.TimedOut) { Add-SanityCheck -Report $report -Id $id -Title $title -Skipped -SkipReason 'govulncheck timed out'; continue }
  $findings = @(($r.StdOut + "`n" + $r.StdErr) -split "`r?`n" | Where-Object { $_ -match '^Vulnerability #|GO-\d{4}-\d+' } | Select-Object -Unique)
  if ($r.ExitCode -ne 0 -and $r.ExitCode -ne 3 -and $findings.Count -eq 0) { Add-SanityCheck -Report $report -Id $id -Title $title -Skipped -SkipReason ("govulncheck exited {0}: {1}" -f $r.ExitCode, ((($r.StdErr + "`n" + $r.StdOut) -split "`r?`n" | Where-Object { $_.Trim() } | Select-Object -First 10) -join ' | ')); continue }
  Add-SanityCheck -Report $report -Id $id -Title $title -Hits $findings -Mode strict -Severity high -Remediation 'Upgrade the affected module to the fixed version named by govulncheck.' -Command "cd $modDir; govulncheck ./..."
}

Add-SanityNote -Report $report -Text 'This check is informational: only confirmed vulnerabilities fail it. The "What changed" column must be filled from upstream release notes before a recommendation is final.'
if ($SkipNetwork) { Add-SanityNote -Report $report -Text 'All package-manager checks were skipped (-SkipNetwork).' }

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
