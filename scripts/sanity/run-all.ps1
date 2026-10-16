# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Runs every McHarbor sanity check (scripts/sanity/<check>/check.ps1) and writes
  one report per check plus a summary to .results/sanity/.
.DESCRIPTION
  Each check mirrors one file in .instructions/sanity/. Checks are pure
  PowerShell 5.1 and read-only; they never modify project files.

  Exit code: 0 when nothing failed, 1 when at least one check reports "fail"
  (or "review" with -FailOn review), 2 when a check script itself crashed.
.PARAMETER Check
  One or more check names to run (folder names). Default: all.
.PARAMETER List
  List the available checks and exit.
.PARAMETER SkipNetwork
  Skip npm / go network calls (deprecated and dependencies checks).
.PARAMETER FailOn
  fail (default): exit 1 only on "fail". review: exit 1 on "review" too. never: always exit 0.
.PARAMETER OutDir
  Report directory, relative to the repo root. Default .results/sanity.
.EXAMPLE
  .\scripts\sanity\run-all.ps1
.EXAMPLE
  .\scripts\sanity\run-all.ps1 -Check security,permissions -SkipNetwork
.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File scripts/sanity/run-all.ps1 -FailOn review
#>
[CmdletBinding()]
param(
  [string[]]$Check,
  [switch]$List,
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [switch]$SkipNetwork,
  [ValidateSet('fail', 'review', 'never')][string]$FailOn = 'fail',
  [int]$MaxHits = 1000,
  [switch]$NoSummaryFile,
  [string[]]$Paths
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir (Join-Path $PSScriptRoot 'x')   # x/../../.. == repo root
if (-not $Stamp) { $Stamp = Get-SanityStamp }

# Canonical order (matches .instructions/sanity/README.md); extra folders are appended alphabetically.
$order = @('security', 'performance', 'code-quality', 'modularity', 'ui-consistency', 'error-visualization', 'copyright', 'code-comments', 'i18n', 'i18n-untranslated', 'go-standards', 'react-standards', 'deprecated', 'dependencies', 'app-catalog', 'orphaned-backend', 'orphaned-frontend', 'missing-api', 'permissions', 'plugin-host-coupling', 'plugin-isolation', 'duplication')
$available = @(Get-ChildItem -LiteralPath $PSScriptRoot -Directory | Where-Object { $_.Name -notmatch '^_' -and (Test-Path -LiteralPath (Join-Path $_.FullName 'check.ps1')) } | ForEach-Object { $_.Name })
$ordered = @($order | Where-Object { $available -contains $_ }) + @($available | Where-Object { $order -notcontains $_ } | Sort-Object)

if ($List) {
  Write-Host 'Available sanity checks:' -ForegroundColor White
  foreach ($c in $ordered) { Write-Host ("  {0,-22} scripts/sanity/{0}/check.ps1   <- .instructions/sanity/{0}.md" -f $c) }
  exit 0
}

$selected = $ordered
if ($Check -and $Check.Count) {
  $selected = @()
  foreach ($c in $Check) {
    foreach ($part in ($c -split ',')) {
      $name = $part.Trim()
      if (-not $name) { continue }
      if ($available -notcontains $name) { Write-Host "Unknown check '$name'. Use -List to see the available checks." -ForegroundColor Red; exit 2 }
      if ($selected -notcontains $name) { $selected += $name }
    }
  }
  $selected = @($ordered | Where-Object { $selected -contains $_ })
}

$outFull = Get-SanityOutDir -Root $Root -OutDir $OutDir
Write-Host ''
Write-Host ("McHarbor sanity suite - {0} check(s), stamp {1}" -f $selected.Count, $Stamp) -ForegroundColor White
Write-Host ("root:    {0}" -f $Root) -ForegroundColor DarkGray
Write-Host ("reports: {0}" -f $outFull) -ForegroundColor DarkGray
if ($SkipNetwork) { Write-Host 'network checks: skipped (-SkipNetwork)' -ForegroundColor DarkGray }

$results = @()
$crashed = 0
$suiteStart = Get-Date
foreach ($name in $selected) {
  $script = Join-Path (Join-Path $PSScriptRoot $name) 'check.ps1'
  $params = @{ Root = $Root; OutDir = $outFull; Stamp = $Stamp; MaxHits = $MaxHits }
  $cmd = Get-Command $script
  if ($SkipNetwork -and $cmd.Parameters.ContainsKey('SkipNetwork')) { $params['SkipNetwork'] = $true }
  if ($Paths -and $Paths.Count -and $cmd.Parameters.ContainsKey('Paths')) { $params['Paths'] = $Paths }
  # A full (unscoped) run lets a check shrink its own baseline; a scoped run never sees every baselined file.
  if (-not ($Paths -and $Paths.Count) -and $cmd.Parameters.ContainsKey('PruneBaseline')) { $params['PruneBaseline'] = $true }
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  try {
    $out = @(& $script @params)
    $summary = $out | Where-Object { $null -ne $_ -and $_.PSObject.Properties['Check'] } | Select-Object -Last 1
    if ($null -eq $summary) { throw 'check produced no summary object' }
    $results += $summary
  } catch {
    $crashed++
    Write-Host ("   [ERROR ] {0}: {1} (line {2})" -f $name, $_.Exception.Message, $_.InvocationInfo.ScriptLineNumber) -ForegroundColor Red
    $results += [pscustomobject]@{ Check = $name; Status = 'error'; Fail = 0; Review = 0; Info = 0; Pass = 0; Skipped = 0; Hits = 0; Seconds = [Math]::Round($sw.Elapsed.TotalSeconds, 1); Report = ''; Error = $_.Exception.Message }
  }
}

# ---- Console summary ----
Write-Host ''
Write-Host '== Summary ==' -ForegroundColor White
$fmt = '{0,-22} {1,-7} {2,5} {3,7} {4,5} {5,5} {6,8} {7,6} {8,8}'
Write-Host ($fmt -f 'check', 'status', 'fail', 'review', 'info', 'pass', 'skipped', 'hits', 'seconds') -ForegroundColor DarkGray
foreach ($r in $results) {
  $color = switch ($r.Status) { 'pass' { 'Green' } 'fail' { 'Red' } 'review' { 'Yellow' } 'error' { 'Magenta' } default { 'Gray' } }
  Write-Host ($fmt -f $r.Check, $r.Status.ToUpperInvariant(), $r.Fail, $r.Review, $r.Info, $r.Pass, $r.Skipped, $r.Hits, $r.Seconds) -ForegroundColor $color
}
$totalFail = @($results | Where-Object { $_.Status -eq 'fail' }).Count
$totalReview = @($results | Where-Object { $_.Status -eq 'review' }).Count
$totalPass = @($results | Where-Object { $_.Status -eq 'pass' }).Count
$elapsed = [Math]::Round(((Get-Date) - $suiteStart).TotalSeconds, 1)
Write-Host ''
Write-Host ("checks: {0} failed, {1} need review, {2} passed, {3} crashed - {4}s" -f $totalFail, $totalReview, $totalPass, $crashed, $elapsed) -ForegroundColor $(if ($totalFail -or $crashed) { 'Red' } elseif ($totalReview) { 'Yellow' } else { 'Green' })

# ---- Summary file ----
if (-not $NoSummaryFile) {
  $md = New-Object System.Collections.Generic.List[string]
  $md.Add('# Sanity Suite Summary')
  $md.Add('')
  $md.Add("Date: $((Get-Date).ToString('yyyy-MM-dd HH:mm'))")
  $md.Add("Root: ``$Root``")
  $md.Add("Checks: $($selected -join ', ')")
  if ($SkipNetwork) { $md.Add('Network checks skipped (-SkipNetwork).') }
  $md.Add('')
  $md.Add('| Check | Status | Fail | Review | Info | Pass | Skipped | Hits | Seconds | Report |')
  $md.Add('|---|---|---|---|---|---|---|---|---|---|')
  foreach ($r in $results) {
    $link = if ($r.Report) { "[$([System.IO.Path]::GetFileName($r.Report))]($([System.IO.Path]::GetFileName($r.Report)))" } else { "error: $($r.Error)" }
    $md.Add(("| {0} | **{1}** | {2} | {3} | {4} | {5} | {6} | {7} | {8} | {9} |" -f $r.Check, $r.Status, $r.Fail, $r.Review, $r.Info, $r.Pass, $r.Skipped, $r.Hits, $r.Seconds, $link))
  }
  $md.Add('')
  $md.Add("Totals: $totalFail failed, $totalReview need review, $totalPass passed, $crashed crashed; ${elapsed}s.")
  $md.Add('')
  $md.Add('Verdicts: `fail` = unambiguous violation; `review` = hits a human must confirm or dismiss; `pass` = no hits in any check; `info`/`skipped` counts are per-check inventories and skipped prerequisites.')
  $summaryPath = Join-Path $outFull "$Stamp-summary.md"
  [System.IO.File]::WriteAllLines($summaryPath, $md.ToArray(), (New-Object System.Text.UTF8Encoding($false)))
  Write-Host ("summary: {0}" -f (Get-SanityRelPath -Root $Root -Full $summaryPath)) -ForegroundColor DarkGray
}

if ($crashed -gt 0) { exit 2 }
switch ($FailOn) {
  'never' { exit 0 }
  'review' { if ($totalFail -or $totalReview) { exit 1 } else { exit 0 } }
  default { if ($totalFail) { exit 1 } else { exit 0 } }
}
