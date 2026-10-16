# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Plugin isolation sanity check. Mirrors .instructions/sanity/plugin-isolation.md.
.DESCRIPTION
  Enforces decision D-1/D-2 of
  .references/todo/2026-09-27-deduplication-and-authoring-guardrails.md: no plugin
  depends on another plugin, and none depends on shared plugin library code.

  This is the plugin -> plugin direction. The host -> plugin direction (the host
  hardcoding a slug) is plugin-host-coupling, and the two do not overlap.

  Why it matters: a package linked into N .so files couples their release cycles
  and their ABI. Per .claude/rules/versioning.md a change to shared code needs
  every consumer rebuilt and reinstalled through the Store's Update path, to keep
  the four-way agreement between the DB row, the install directory, plugin.yaml
  and the .so Version symbol. One typo in a shared client would mean rebuilding
  twelve plugins.

  The baseline is clean, so this check starts green and any hit is a real
  regression. Do not add an exemption; move the shared code back into each
  plugin.

  Check 4 reports the host contract surface rather than failing on it.
  core/plugin and core/plugin/specs/* are the API and cannot be removed, but
  growth there is worth seeing: per the saved note
  shared-spec-edit-breaks-every-plugin, touching core/plugin/specs/** unloads
  every installed .so.
.EXAMPLE
  .\scripts\sanity\plugin-isolation\check.ps1
.EXAMPLE
  .\scripts\sanity\plugin-isolation\check.ps1 -Paths addon-kvm,addon-swarm
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [string[]]$Paths,
  [switch]$Changed
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$addonsModule = 'github.com/therealmcsparrow/mcharbor-addons/plugins'
$hostModule = 'github.com/therealmcsparrow/mcharbor'
$expectedReplace = '../../../backend'

$pluginRoot = Join-Path $Root 'src\plugins\Plugins'
$report = New-SanityReport -Name 'plugin-isolation' -Title 'Plugin Isolation Sanity' -Scope 'src/plugins/Plugins/*' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

if (-not (Test-Path -LiteralPath $pluginRoot)) {
  Add-SanityCheck -Report $report -Id 1 -Title 'Cross-plugin Go imports' -Skipped -SkipReason 'src/plugins/Plugins is absent'
  $summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
  $summary
  exit 0
}

$plugins = @(Get-ChildItem -LiteralPath $pluginRoot -Directory | Sort-Object Name)
if ($Changed) {
  $touched = @(& git -C $Root status --porcelain 2>$null |
    ForEach-Object { ($_ -replace '^...', '') -replace '^"|"$', '' } |
    Where-Object { $_ -match '^src/plugins/Plugins/([^/]+)/' } |
    ForEach-Object { $Matches[1] } | Sort-Object -Unique)
  $plugins = @($plugins | Where-Object { $touched -contains $_.Name })
}
if ($Paths) { $plugins = @($plugins | Where-Object { $Paths -contains $_.Name }) }

# Every import line in every plugin, tagged with the plugin it belongs to. One
# pass, because three of the four checks read the same lines.
$crossImports = New-Object System.Collections.Generic.List[object]
$sdkImports = New-Object System.Collections.Generic.List[object]
$badReplaces = New-Object System.Collections.Generic.List[object]
$hostSurface = @{}
$goFileCount = 0

foreach ($plugin in $plugins) {
  $slug = $plugin.Name
  $ownPrefix = "$addonsModule/$slug"

  foreach ($file in (Get-ChildItem -LiteralPath $plugin.FullName -Recurse -File -Filter '*.go' -ErrorAction SilentlyContinue)) {
    if ($file.FullName -match '[\\/](node_modules|vendor|dist)[\\/]') { continue }
    $goFileCount++
    $rel = Get-SanityRelPath -Root $Root -Full $file.FullName
    $lines = Get-SanityLines $file.FullName
    for ($i = 0; $i -lt $lines.Length; $i++) {
      $line = $lines[$i]
      if ($line -notmatch [regex]::Escape($hostModule) -and $line -notmatch 'plugin-sdk') { continue }

      # 1. An import of another plugin's module path.
      if ($line -match ([regex]::Escape($addonsModule) + '/([a-z0-9][a-z0-9-]*)')) {
        $target = $Matches[1]
        if ($target -ne $slug) {
          $crossImports.Add((New-SanityHit -File $rel -Line ($i + 1) -Text ("{0} imports {1}: {2}" -f $slug, $target, $line.Trim())))
        }
      }

      # 3. Any plugin-sdk path. There is none today; this is what keeps it that way.
      if ($line -match 'plugin-sdk') {
        $sdkImports.Add((New-SanityHit -File $rel -Line ($i + 1) -Text ("{0}: {1}" -f $slug, $line.Trim())))
      }

      # 4. The host contract surface, reported not failed.
      if ($line -match ('"' + [regex]::Escape($hostModule) + '/([a-z0-9/_-]+)"')) {
        $pkg = "$hostModule/$($Matches[1])"
        if ($pkg -notlike "$addonsModule*") {
          if (-not $hostSurface.ContainsKey($pkg)) { $hostSurface[$pkg] = 0 }
          $hostSurface[$pkg]++
        }
      }
    }
  }

  # 2. The replace directive. Exactly one, pointing at the host backend.
  $goMod = Join-Path $plugin.FullName 'go.mod'
  if (Test-Path -LiteralPath $goMod) {
    $modLines = Get-SanityLines $goMod
    $seen = 0
    for ($i = 0; $i -lt $modLines.Length; $i++) {
      $line = $modLines[$i]
      if ($line -notmatch '=>') { continue }
      $seen++
      $target = ($line -split '=>')[-1].Trim()
      if ($target -ne $expectedReplace) {
        $badReplaces.Add((New-SanityHit -File (Get-SanityRelPath -Root $Root -Full $goMod) -Line ($i + 1) `
          -Text ("{0}: replace target is '{1}', expected '{2}'" -f $slug, $target, $expectedReplace)))
      }
    }
    if ($seen -gt 1) {
      $badReplaces.Add((New-SanityHit -File (Get-SanityRelPath -Root $Root -Full $goMod) -Line 0 `
        -Text ("{0}: {1} replace directives, expected exactly 1" -f $slug, $seen)))
    }
  }
}

Add-SanityCheck -Report $report -Id 1 -Title 'Cross-plugin Go imports' -Hits $crossImports.ToArray() -Mode strict -Severity high `
  -Remediation 'A plugin is a self-contained Go module (D-1). Copy the code into the importing plugin; do not share it.' `
  -Note ("{0} plugin(s), {1} .go file(s) scanned. An import of {2}/<slug> is only legal when <slug> is the importing plugin's own; addon-swarm and addon-reporting import their own subpackages and are correct." -f $plugins.Count, $goFileCount, $addonsModule) `
  -Command 'rg -n "mcharbor-addons/plugins/" src/plugins/Plugins'

Add-SanityCheck -Report $report -Id 2 -Title 'Plugin go.mod replace directives' -Hits $badReplaces.ToArray() -Mode strict -Severity high `
  -Remediation ("Each plugin go.mod carries exactly one replace, '{0}'. A second one is a path by which a cross-plugin dependency could exist." -f $expectedReplace) `
  -Command 'rg -n "=>" src/plugins/Plugins/*/go.mod'

Add-SanityCheck -Report $report -Id 3 -Title 'Imports of a shared plugin SDK' -Hits $sdkImports.ToArray() -Mode strict -Severity high `
  -Remediation 'There is no plugin SDK, by decision D-1. Shared library code between plugins couples their release cycles and their ABI.' `
  -Command 'rg -n "plugin-sdk" src/plugins'

$surfaceRows = @($hostSurface.GetEnumerator() | Sort-Object -Property Value -Descending | ForEach-Object {
  $kind = if ($_.Key -like "$hostModule/core/plugin*") { 'host plugin contract' } else { 'host package' }
  , @($_.Key, $_.Value, $kind)
})
Add-SanityCheck -Report $report -Id 4 -Title 'Host contract surface the plugins import' -Hits @() -Mode info -Severity info `
  -Note ("{0} distinct host package(s). This is the one shared surface that genuinely cannot be removed, so it is reported rather than failed. Growth here is the thing to watch: touching core/plugin/specs/** unloads every installed .so." -f $surfaceRows.Count) `
  -Table @{ Headers = @('Package', 'Imports', 'Kind'); Rows = $surfaceRows } `
  -Command 'rg -no "github.com/therealmcsparrow/mcharbor/core/[a-z/]*" src/plugins/Plugins | sort -u'

Add-SanityNote -Report $report -Text 'Zero hits on checks 1-3 means: OK, every plugin is self-contained. The baseline is clean, so any hit is a regression and not a backlog item.'
Add-SanityNote -Report $report -Text 'Decisions: D-1 no shared code between plugins; D-2 the six addon-* plugins are absolute and their duplication is accepted permanently. See .references/todo/2026-09-27-deduplication-and-authoring-guardrails.md.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
