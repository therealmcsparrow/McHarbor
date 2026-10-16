# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Orphaned frontend sanity check. Mirrors .instructions/sanity/orphaned-frontend.md.
.DESCRIPTION
  Resolves every import specifier in src/frontend, widgets and nodes (relative
  paths and tsconfig aliases) to a file, then reports files nothing imports,
  exported components/hooks with no consumers, and barrel re-exports nobody uses.
.EXAMPLE
  .\scripts\sanity\orphaned-frontend\check.ps1
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$report = New-SanityReport -Name 'orphaned-frontend' -Title 'Orphaned Frontend Sanity' -Scope 'src/frontend, widgets, nodes' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$feRoot = Join-Path $Root 'src\frontend'
$scan = @{ Root = $Root; Paths = @('src/frontend', 'widgets', 'nodes'); Include = @('*.ts', '*.tsx', '*.js', '*.jsx', '*.mjs', '*.cjs') }
$files = @(Get-SanityFiles @scan)
$byKey = @{}
foreach ($f in $files) { $byKey[$f.Full.ToLowerInvariant()] = $f }

# tsconfig path aliases -> absolute directory prefixes
$aliases = @()
$tsconfig = Join-Path $feRoot 'tsconfig.json'
if (Test-Path -LiteralPath $tsconfig) {
  $tsText = Get-SanityText $tsconfig
  foreach ($m in [regex]::Matches($tsText, '"([^"]+?)(/\*)?"\s*:\s*\[\s*"([^"]+?)(/\*)?"\s*\]')) {
    $alias = $m.Groups[1].Value; $target = $m.Groups[3].Value
    if ($alias -match '^(react|@tabler|@tanstack)' -or $target -match 'node_modules') { continue }
    $abs = [System.IO.Path]::GetFullPath((Join-Path $feRoot $target))
    $aliases += [pscustomobject]@{ Alias = $alias; Wild = [bool]$m.Groups[2].Success; Target = $abs }
  }
}
$aliases = @($aliases | Sort-Object { $_.Alias.Length } -Descending)
$exts = @('.ts', '.tsx', '.d.ts', '.js', '.jsx', '.mjs', '.json', '.css', '.svg', '.png', '.md')

function Resolve-SanityImport([string]$spec, [string]$fromDir) {
  $spec = $spec -replace '\?.*$', ''
  if (-not $spec) { return $null }
  $base = $null
  if ($spec.StartsWith('.')) { $base = Join-Path $fromDir $spec }
  else {
    foreach ($a in $aliases) {
      if ($a.Wild -and $spec.StartsWith($a.Alias + '/')) { $base = Join-Path $a.Target $spec.Substring($a.Alias.Length + 1); break }
      if (-not $a.Wild -and $spec -eq $a.Alias) { $base = $a.Target; break }
      if ($a.Wild -and $spec -eq $a.Alias) { $base = $a.Target; break }
    }
  }
  if (-not $base) { return $null }
  try { $base = [System.IO.Path]::GetFullPath($base) } catch { return $null }
  $candidates = @($base) + @($exts | ForEach-Object { "$base$_" }) + @('index.ts', 'index.tsx', 'index.js') | ForEach-Object { if ($_ -match '^index\.') { Join-Path $base $_ } else { $_ } }
  foreach ($c in $candidates) {
    $k = $c.ToLowerInvariant()
    if ($byKey.ContainsKey($k)) { return $k }
    if (($c -match '\.(json|css|svg|png|md)$') -and (Test-Path -LiteralPath $c -PathType Leaf)) { return $k }
  }
  return $null
}

# Build importer graph: target -> set of importer files
$importers = @{}
$specRx = New-Object System.Text.RegularExpressions.Regex("(?:\bfrom\s*|\bimport\s*\(\s*|\brequire\s*\(\s*|^\s*import\s+)['`"]([^'`"]+)['`"]", [System.Text.RegularExpressions.RegexOptions]::Multiline)
# `<[^>]*>` stopped at the first `>`, so a nested type argument —
# `import.meta.glob<Record<string, unknown>>(...)`, which is how the node and
# widget registries are written — never matched, and every file those globs
# load read as an orphan.
$globRx = New-Object System.Text.RegularExpressions.Regex("import\.meta\.glob[^(]*\(\s*\[?\s*['`"]([^'`"]+)['`"]")
$globPatterns = @()
foreach ($f in $files) {
  $text = Get-SanityText $f.Full
  $fromDir = [System.IO.Path]::GetDirectoryName($f.Full)
  foreach ($m in $specRx.Matches($text)) {
    $target = Resolve-SanityImport $m.Groups[1].Value $fromDir
    if (-not $target) { continue }
    if (-not $importers.ContainsKey($target)) { $importers[$target] = New-Object 'System.Collections.Generic.HashSet[string]' }
    $importers[$target].Add($f.Full.ToLowerInvariant()) | Out-Null
  }
  foreach ($m in $globRx.Matches($text)) { $globPatterns += [pscustomobject]@{ Pattern = $m.Groups[1].Value; FromDir = $fromDir } }
}
# import.meta.glob patterns mark their matches as imported
$globRegexes = @()
foreach ($g in $globPatterns) {
  $p = $g.Pattern
  $resolvedBase = $null
  foreach ($a in $aliases) { if ($a.Wild -and $p.StartsWith($a.Alias + '/')) { $resolvedBase = Join-Path $a.Target $p.Substring($a.Alias.Length + 1); break } }
  if (-not $resolvedBase -and $p.StartsWith('.')) { $resolvedBase = Join-Path $g.FromDir $p }
  if (-not $resolvedBase) { continue }
  $rx = [regex]::Escape(($resolvedBase -replace '/', '\')) -replace '\\\*\\\*', '.*' -replace '\\\*', '[^\\]*'
  $globRegexes += New-Object System.Text.RegularExpressions.Regex(('^' + $rx + '$'), [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
}
# index.html entry points
$entryPoints = New-Object 'System.Collections.Generic.HashSet[string]'
$indexHtml = Join-Path $feRoot 'index.html'
if (Test-Path -LiteralPath $indexHtml) {
  foreach ($m in [regex]::Matches((Get-SanityText $indexHtml), 'src="([^"]+)"')) {
    $p = $m.Groups[1].Value.TrimStart('/')
    $abs = [System.IO.Path]::GetFullPath((Join-Path $feRoot $p)); $entryPoints.Add($abs.ToLowerInvariant()) | Out-Null
  }
}

# 1. Files nothing imports
# core/pwa/sw.ts is the Vite service-worker lib entry and a knip entry; knip-external-consumers.ts is a knip entry;
# lint/mcharbor-plugin.js is loaded by name from the Oxlint config, which the import scan does not follow.
$entryRx = '(^|/)(main\.tsx|vite-env\.d\.ts|vite\.config\.[cm]?[jt]s|vitest\.config\.[cm]?[jt]s|tailwind\.config\.[cm]?[jt]s|postcss\.config\.[cm]?[jt]s|eslint\.config\.[cm]?[jt]s|playwright(\.\w+)?\.config\.[cm]?[jt]s|ladle\.config\.[cm]?[jt]s|\.ladle/.*|tests?/.*|e2e/.*|__tests__/.*|scripts/.*|core/pwa/sw.ts|knip-external-consumers.ts|lint/mcharbor-plugin.js)$|\.d\.ts$|\.(test|spec|stories|visual\.spec)\.[jt]sx?$|\.setup\.[jt]s$'
$orphanFiles = @()
foreach ($f in $files) {
  $k = $f.Full.ToLowerInvariant()
  if ($f.Rel -match $entryRx) { continue }
  if ($entryPoints.Contains($k)) { continue }
  if ($importers.ContainsKey($k) -and $importers[$k].Count -gt 0) { continue }
  $globbed = $false; foreach ($g in $globRegexes) { if ($g.IsMatch($f.Full)) { $globbed = $true; break } }
  if ($globbed) { continue }
  $orphanFiles += New-SanityHit -File $f.Rel -Line 0 -Text 'no file imports this module'
}
Add-SanityCheck -Report $report -Id 1 -Title 'TS/TSX files nothing imports' -Hits $orphanFiles -Mode review -Severity medium `
  -Remediation 'Delete after confirming the file is not loaded by string key (router registry, plugin registry) or by a Vite glob the scan did not see.' `
  -Note ("{0} files scanned, {1} tsconfig aliases, {2} import.meta.glob patterns honoured, {3} index.html entry points." -f $files.Count, $aliases.Count, $globRegexes.Count, $entryPoints.Count)

# 2. Exported components / hooks with no consumers
$tokens = Get-SanityTokenCounts -Files $files
$exportRx = New-Object System.Text.RegularExpressions.Regex('(?m)^export\s+(?:async\s+)?(?:function|const|class)\s+([A-Z]\w*|use[A-Z]\w*)\b')
$unusedExports = @()
foreach ($f in $files) {
  if ($f.Rel -match '\.(test|spec|stories)\.[jt]sx?$|\.d\.ts$|resources/utils/|core/utils/') { continue }
  $text = Get-SanityText $f.Full
  foreach ($m in $exportRx.Matches($text)) {
    $name = $m.Groups[1].Value
    $count = 0; if ($tokens.ContainsKey($name)) { $count = $tokens[$name] }
    if ($count -le 1) { $unusedExports += New-SanityHit -File $f.Rel -Line (Get-SanityLineNumber -Text $text -Index $m.Index) -Text "$name - exported, referenced nowhere else" }
  }
}
Add-SanityCheck -Report $report -Id 2 -Title 'Exported components / hooks with no consumers' -Hits $unusedExports -Mode review -Severity low `
  -Remediation 'Consider deleting; names looked up by string key (router, plugin registry) are false positives.' -Note 'Token counts span src/frontend, widgets and nodes; resources/utils and core/utils are deliberately public and excluded.'

# 3. Barrel index.ts re-exports with no consumers
$barrelOrphans = @()
foreach ($b in @($files | Where-Object { $_.Name -eq 'index.ts' -or $_.Name -eq 'index.tsx' })) {
  $bKey = $b.Full.ToLowerInvariant()
  $barrelImported = $importers.ContainsKey($bKey) -and $importers[$bKey].Count -gt 0
  $text = Get-SanityText $b.Full
  foreach ($m in [regex]::Matches($text, "(?m)^export\s+(?:\*|\{[^}]*\}|type\s+\{[^}]*\})\s+from\s+['`"]([^'`"]+)['`"]")) {
    $target = Resolve-SanityImport $m.Groups[1].Value ([System.IO.Path]::GetDirectoryName($b.Full))
    if (-not $target) { continue }
    $others = 0
    if ($importers.ContainsKey($target)) { $others = @($importers[$target] | Where-Object { $_ -ne $bKey }).Count }
    if ($others -eq 0 -and -not $barrelImported) { $barrelOrphans += New-SanityHit -File $b.Rel -Line (Get-SanityLineNumber -Text $text -Index $m.Index) -Text ("re-exports {0}; neither the barrel nor the target is imported anywhere" -f $m.Groups[1].Value) }
  }
}
Add-SanityCheck -Report $report -Id 3 -Title 'Barrel re-exports with no consumers' -Hits $barrelOrphans -Mode review -Severity medium `
  -Remediation 'Remove the re-export or the barrel. Public-surface barrels (resources/components/ui/index.ts) and router-registered module barrels are accepted.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
