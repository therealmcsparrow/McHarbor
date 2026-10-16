# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Deprecated usage sanity check. Mirrors .instructions/sanity/deprecated.md.
.DESCRIPTION
  Source-level deprecation markers, npm deprecation warnings, stale majors,
  Go module Deprecated/Retracted flags, Compose/Dockerfile legacy syntax and
  deprecated browser/React APIs. Checks 2-4 call npm / go and need network
  access; pass -SkipNetwork to skip them.
.EXAMPLE
  .\scripts\sanity\deprecated\check.ps1 -SkipNetwork
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

$report = New-SanityReport -Name 'deprecated' -Title 'Deprecated Usage Sanity' -Scope 'src, docker, docker-compose*.yml, package.json, go.mod' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$frontendDir = Join-Path $Root 'src\frontend'
$npm = Resolve-SanityTool 'npm'
$go = Resolve-SanityTool 'go'
$env:GOTOOLCHAIN = 'auto'
$env:GOWORK = 'off'

# 1. Source-level deprecation markers
#
# A marker is this repo declaring one of its own symbols deprecated:
# `@deprecated` or a `Deprecated:` line. Prose that mentions somebody else's
# deprecation — "the deprecated gateway4 field", "//nolint:staticcheck //
# clearing the deprecated field is the point" — is a note about an upstream
# API, and there is nothing here to migrate. That is inventory, not a finding.
$markers = @(Find-Pattern -Root $Root -Paths @('src', 'docker') -Include @('*.go', '*.ts', '*.tsx', '*.yml', '*.yaml', 'Dockerfile*') -ExcludeMatch @('_test\.go$', '\.test\.tsx?$') -Pattern '@deprecated|^\s*(//|\*|#)\s*Deprecated:')
$mentions = @(Find-Pattern -Root $Root -Paths @('src', 'docker') -Include @('*.go', '*.ts', '*.tsx', '*.yml', '*.yaml', 'Dockerfile*') -ExcludeMatch @('_test\.go$', '\.test\.tsx?$') -IgnoreCase -Pattern '\bdeprecat(ed|ion)\b' -NotMatch '@deprecated|^\s*(//|\*|#)\s*Deprecated:')
$composeMarkers = @(Find-Pattern -Root $Root -Paths @('docker-compose.yml', 'docker-compose.dev.yml', 'docker-compose.ha.yml', 'docker-compose.secrets.yml') -IgnoreCase -Pattern '\bdeprecated\b')
Add-SanityCheck -Report $report -Id 1 -Title 'Deprecation markers on this repo''s own symbols' -Hits ($markers + $composeMarkers) -Mode review -Severity low `
  -Remediation 'For each: name the replacement and, when compatibility must be kept, a removal timeline. Delete the symbol once nothing calls it.' `
  -Command 'rg -nP "@deprecated|^\s*(//|\*|#)\s*Deprecated:" src docker docker-compose*.yml'
Add-SanityCheck -Report $report -Id '1b' -Title 'Comments mentioning an upstream deprecation (inventory)' -Hits $mentions -Mode info -Severity info `
  -Note 'Notes about another project''s deprecated API. Nothing to migrate here.'

# 2. Frontend package deprecation warnings (npm)
if ($SkipNetwork) {
  Add-SanityCheck -Report $report -Id 2 -Title 'npm deprecation warnings (npm install --dry-run)' -Skipped -SkipReason '-SkipNetwork'
} elseif (-not $npm) {
  Add-SanityCheck -Report $report -Id 2 -Title 'npm deprecation warnings (npm install --dry-run)' -Skipped -SkipReason 'npm not found on PATH'
} else {
  $r = Invoke-SanityCommand -FilePath $npm -Arguments 'install --package-lock-only --dry-run --ignore-scripts --no-audit --no-fund --loglevel warn' -WorkingDirectory $frontendDir -TimeoutSec $CommandTimeoutSec
  if ($r.Failed) {
    Add-SanityCheck -Report $report -Id 2 -Title 'npm deprecation warnings (npm install --dry-run)' -Skipped -SkipReason ("npm failed: " + ($r.StdErr -split "`n" | Select-Object -First 1))
  } else {
    $warns = @(($r.StdOut + "`n" + $r.StdErr) -split "`r?`n" | Where-Object { $_ -match 'npm (WARN|warn) deprecated' } | ForEach-Object { ($_ -replace '^npm (WARN|warn) deprecated\s*', '').Trim() } | Sort-Object -Unique)
    Add-SanityCheck -Report $report -Id 2 -Title 'npm deprecation warnings (npm install --dry-run)' -Hits $warns -Mode review -Severity medium `
      -Remediation 'Direct dependency: upgrade to the named replacement or remove. Transitive: upgrade the parent (npm ls <package> names it).' -Command 'cd src/frontend; npm install --package-lock-only --dry-run'
  }
}

# 3. Frontend stale major versions
function Get-SanityMajor([string]$v) { $m = [regex]::Match("$v", '^\D*(\d+)'); if ($m.Success) { return [int]$m.Groups[1].Value } else { return -1 } }
$pkgJson = $null
$pkgPath = Join-Path $frontendDir 'package.json'
if (Test-Path -LiteralPath $pkgPath) { try { $pkgJson = ConvertFrom-SanityJson (Get-SanityText $pkgPath) } catch { $pkgJson = $null } }
$directDeps = @{}
if ($pkgJson) {
  foreach ($section in @('dependencies', 'devDependencies')) {
    $d = Get-SanityJsonValue $pkgJson $section
    if ($d -is [System.Collections.IDictionary]) { foreach ($k in $d.Keys) { $directDeps[$k] = "$($d[$k])" } }
  }
}
$outdatedJson = $null
if ($SkipNetwork) {
  Add-SanityCheck -Report $report -Id 3 -Title 'Direct npm dependencies a major version behind latest' -Skipped -SkipReason '-SkipNetwork'
} elseif (-not $npm) {
  Add-SanityCheck -Report $report -Id 3 -Title 'Direct npm dependencies a major version behind latest' -Skipped -SkipReason 'npm not found on PATH'
} else {
  $r = Invoke-SanityCommand -FilePath $npm -Arguments 'outdated --json --loglevel error' -WorkingDirectory $frontendDir -TimeoutSec $CommandTimeoutSec
  if ($r.TimedOut -or [string]::IsNullOrWhiteSpace($r.StdOut)) {
    if ($r.TimedOut) { Add-SanityCheck -Report $report -Id 3 -Title 'Direct npm dependencies a major version behind latest' -Skipped -SkipReason 'npm outdated timed out' }
    else { Add-SanityCheck -Report $report -Id 3 -Title 'Direct npm dependencies a major version behind latest' -Hits @() -Mode review -Severity low -Note 'npm outdated printed nothing: no outdated packages.' }
  } else {
    try { $outdatedJson = ConvertFrom-SanityJson $r.StdOut } catch { $outdatedJson = $null }
    $majors = @()
    if ($outdatedJson -is [System.Collections.IDictionary]) {
      foreach ($name in $outdatedJson.Keys) {
        if (-not $directDeps.ContainsKey($name)) { continue }
        $e = $outdatedJson[$name]
        $cur = "$(Get-SanityJsonValue $e 'current')"; $latest = "$(Get-SanityJsonValue $e 'latest')"
        if ((Get-SanityMajor $cur) -ge 0 -and (Get-SanityMajor $latest) -gt (Get-SanityMajor $cur)) {
          $uses = @(Find-Pattern -Root $Root -Paths @('src/frontend') -Include @('*.ts', '*.tsx', '*.js', '*.mjs') -Pattern ("from ['`"]" + [regex]::Escape($name) + "(/|['`"])|require\(['`"]" + [regex]::Escape($name)))
          $majors += New-SanityHit -File 'src/frontend/package.json' -Line 0 -Text ("{0}: current {1}, latest {2} ({3} import sites)" -f $name, $cur, $latest, $uses.Count)
        }
      }
    }
    Add-SanityCheck -Report $report -Id 3 -Title 'Direct npm dependencies a major version behind latest' -Hits $majors -Mode review -Severity medium `
      -Remediation 'Name the target major, link the call sites that need migration, and consult the upstream migration guide. Pinned versions with a documented reason and packages unused by app code are accepted.' -Command 'cd src/frontend; npm outdated --json'
  }
}

# 4. Go module deprecations and retractions
function Read-SanityGoModList([string]$out) {
  $objs = @(); $buf = New-Object System.Text.StringBuilder; $depth = 0
  foreach ($line in ($out -split "`r?`n")) {
    $opens = ([regex]::Matches($line, '\{')).Count; $closes = ([regex]::Matches($line, '\}')).Count
    [void]$buf.AppendLine($line)
    $depth += $opens - $closes
    if ($depth -eq 0 -and $buf.Length -gt 2) {
      $txt = $buf.ToString().Trim()
      if ($txt) { try { $objs += ConvertFrom-SanityJson $txt } catch { } }
      [void]$buf.Clear()
    }
  }
  return $objs
}
foreach ($modDir in @('src/backend', 'src/agent')) {
  $id = if ($modDir -eq 'src/backend') { '4a' } else { '4b' }
  $title = "Go module Deprecated/Retracted flags ($modDir)"
  if ($SkipNetwork) { Add-SanityCheck -Report $report -Id $id -Title $title -Skipped -SkipReason '-SkipNetwork'; continue }
  if (-not $go) { Add-SanityCheck -Report $report -Id $id -Title $title -Skipped -SkipReason 'go not found on PATH'; continue }
  $r = Invoke-SanityCommand -FilePath $go -Arguments 'list -m -u -json all' -WorkingDirectory (Join-Path $Root $modDir) -TimeoutSec $CommandTimeoutSec
  if ($r.Failed -or [string]::IsNullOrWhiteSpace($r.StdOut)) { Add-SanityCheck -Report $report -Id $id -Title $title -Skipped -SkipReason ("go list failed: " + (($r.StdErr -split "`n") | Select-Object -First 1)); continue }
  $direct = @(); $indirect = @()
  foreach ($m in (Read-SanityGoModList $r.StdOut)) {
    if (Get-SanityJsonValue $m 'Main') { continue }
    $path = "$(Get-SanityJsonValue $m 'Path')"; $ver = "$(Get-SanityJsonValue $m 'Version')"
    $dep = Get-SanityJsonValue $m 'Deprecated'; $ret = Get-SanityJsonValue $m 'Retracted'
    if (-not $dep -and -not $ret) { continue }
    $why = @(); if ($dep) { $why += "Deprecated: " + ("$dep" -replace '\s+', ' ') }; if ($ret) { $why += "Retracted: " + (@($ret) -join '; ') }
    $hit = New-SanityHit -File "$modDir/go.mod" -Line 0 -Text ("{0}@{1} - {2}" -f $path, $ver, ($why -join ' | '))
    if (Get-SanityJsonValue $m 'Indirect') { $indirect += $hit } else { $direct += $hit }
  }
  Add-SanityCheck -Report $report -Id $id -Title $title -Hits $direct -Mode strict -Severity medium -Remediation 'Quote the module''s own deprecation text, move to the replacement path/version, and update import paths when the module path changed.' -Note ("{0} indirect modules also carry a flag (informational; check `go mod why -m <module>`)." -f $indirect.Count) -Command "cd $modDir; go list -m -u -json all"
  if ($indirect.Count) { Add-SanityCheck -Report $report -Id "$id-indirect" -Title "Indirect Go modules with Deprecated/Retracted flags ($modDir)" -Hits $indirect -Mode info -Severity info -Note 'Accepted when go mod why reports the main module does not need the module.' }
}

# 5. Go source deprecation markers
$goDep = @(Find-Pattern -Root $Root -Paths @('src/backend', 'src/agent') -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern 'Deprecated:|\b[A-Za-z0-9_]+Deprecated\b|\bDeprecated[A-Za-z0-9_]+\b')
Add-SanityCheck -Report $report -Id 5 -Title 'Go deprecation markers (local APIs and dependency symbols)' -Hits $goDep -Mode review -Severity low -Remediation 'Name the replacement symbol and the smallest safe migration path.' -Command 'rg -n "Deprecated:" src/backend src/agent'

# 6. Docker Compose and Dockerfile deprecated syntax
$composeFiles = @('docker-compose.yml', 'docker-compose.dev.yml', 'docker-compose.ha.yml', 'docker-compose.secrets.yml')
$composeVersion = @(Find-Pattern -Root $Root -Paths $composeFiles -Pattern '^\s*version\s*:')
$composeLinks = @(Find-Pattern -Root $Root -Paths $composeFiles -Pattern '^\s*links\s*:|--link\b')
$dockerfileLegacy = @(Find-Pattern -Root $Root -Paths @('docker') -Include @('Dockerfile*') -Pattern '^\s*MAINTAINER\b|^\s*ENV\s+[^=\s]+\s+[^=\s]+')
$containerName = @(Find-Pattern -Root $Root -Paths $composeFiles -Pattern '^\s*container_name\s*:')
Add-SanityCheck -Report $report -Id '6a' -Title 'Compose version:/links: and Dockerfile MAINTAINER / two-arg ENV' -Hits ($composeVersion + $composeLinks + $dockerfileLegacy) -Mode strict -Severity medium `
  -Remediation 'Drop version:, replace links with networks, use LABEL maintainer=..., use ENV key=value.' -Command 'rg -n "^\s*version\s*:|links\s*:|--link\b|MAINTAINER\b|ENV\s+[^=\s]+\s+[^=\s]+" docker-compose*.yml docker'
Add-SanityCheck -Report $report -Id '6b' -Title 'container_name in compose files (inventory)' -Hits $containerName -Mode info -Severity info -Note 'Only a finding when it blocks scaling, reuse or multi-instance installs.'

# 7. Browser and platform APIs
$browser = @(Find-Pattern -Root $Root -Paths @('src/frontend', 'widgets', 'nodes') -Include @('*.ts', '*.tsx') -ExcludeMatch @('\.d\.ts$', '\.test\.tsx?$') -StripStrings -Pattern '\b(findDOMNode|UNSAFE_\w+|componentWillMount|componentWillReceiveProps|componentWillUpdate|document\.execCommand|(?<![.\w])escape|(?<![.\w])unescape|substr|DOMNodeInserted|DOMNodeRemoved)\(' -NotMatch '^\s*//|escapeHtml|escapeRegExp|regexpEscape|escapeAttr')
Add-SanityCheck -Report $report -Id 7 -Title 'Deprecated React, browser and string APIs' -Hits $browser -Mode strict -Severity medium `
  -Remediation 'refs instead of findDOMNode; slice() instead of substr(); navigator.clipboard instead of execCommand; encodeURIComponent instead of escape().' -Command 'rg -nP "\b(findDOMNode|UNSAFE_component|componentWillMount|document\.execCommand|(?<![.\w])escape\(|substr\()" src/frontend'

Add-SanityNote -Report $report -Text 'For every confirmed item the report should carry: file:line, deprecated item, why it matters, replacement, risk (low/medium/high) and whether it is safe now or a planned migration.'
if ($SkipNetwork) { Add-SanityNote -Report $report -Text 'Network-dependent checks 2, 3 and 4 were skipped (-SkipNetwork).' }

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
