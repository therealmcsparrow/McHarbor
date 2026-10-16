# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Orphaned backend sanity check. Mirrors .instructions/sanity/orphaned-backend.md.
.DESCRIPTION
  Go packages nothing imports, exported identifiers with no references,
  modules with a Mount() that main.go never wires, and plugin folders missing
  from the addon index.
.EXAMPLE
  .\scripts\sanity\orphaned-backend\check.ps1
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

$report = New-SanityReport -Name 'orphaned-backend' -Title 'Orphaned Backend Sanity' -Scope 'src/backend (+ src/agent and src/plugins as importers)' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$goModule = 'github.com/therealmcsparrow/mcharbor'
$goModFile = Join-Path $Root 'src\backend\go.mod'
if (Test-Path -LiteralPath $goModFile) { $m = [regex]::Match((Get-SanityText $goModFile), '(?m)^module\s+(\S+)'); if ($m.Success) { $goModule = $m.Groups[1].Value } }

$backendFiles = @(Get-SanityFiles -Root $Root -Paths @('src/backend') -Include @('*.go'))
$backendProd = @($backendFiles | Where-Object { $_.Name -notmatch '_test\.go$' })
$importerFiles = @(Get-SanityFiles -Root $Root -Paths @('src/backend', 'src/agent', 'src/plugins') -Include @('*.go'))

# 1. Packages nothing imports
$imported = New-Object 'System.Collections.Generic.HashSet[string]'
$importRx = New-Object System.Text.RegularExpressions.Regex('"' + [regex]::Escape($goModule) + '/([\w./-]+)"')
foreach ($f in $importerFiles) { foreach ($m in $importRx.Matches((Get-SanityText $f.Full))) { $imported.Add($m.Groups[1].Value) | Out-Null } }
$packages = @{}
foreach ($f in $backendProd) {
  $dir = $f.Dir
  if (-not $packages.ContainsKey($dir)) { $packages[$dir] = @{ Files = 0; Pkg = ''; HasMount = $false; HasDecl = $false } }
  $packages[$dir].Files++
  if (-not $packages[$dir].Pkg) { $pm = [regex]::Match((Get-SanityText $f.Full), '(?m)^package\s+(\w+)'); if ($pm.Success) { $packages[$dir].Pkg = $pm.Groups[1].Value } }
  if ((Get-SanityText $f.Full) -match '(?m)^func Mount\(') { $packages[$dir].HasMount = $true }
  if ((Get-SanityText $f.Full) -match '(?m)^(func|type|var|const)\s') { $packages[$dir].HasDecl = $true }
}
$orphanPkgs = @()
foreach ($dir in ($packages.Keys | Sort-Object)) {
  if ($dir -eq 'src/backend') { continue }                                   # main package
  $importPath = $dir -replace '^src/backend/', ''
  if ($importPath -match '^(cmd/|core/plugin|pluginruntime|core/pluginruntime|_skeleton|tmp_|test/|tests/|testdata)') { continue }
  if ($packages[$dir].Pkg -eq 'main') { continue }
  if ($imported.Contains($importPath)) { continue }
  if ($packages[$dir].HasMount) { continue }                                 # covered by check 3
  # A package whose production files declare nothing (a lone doc.go) exists to
  # hold an env-gated test, so there is nothing for an importer to use.
  if (-not $packages[$dir].HasDecl) { continue }
  $orphanPkgs += New-SanityHit -File $dir -Line 0 -Text ("package {0} ({1} files) is imported by nothing under src/backend, src/agent or src/plugins" -f $packages[$dir].Pkg, $packages[$dir].Files)
}
Add-SanityCheck -Report $report -Id 1 -Title 'Go packages nothing imports' -Hits $orphanPkgs -Mode review -Severity medium `
  -Remediation 'Delete, wire in, or move to _skeleton/ with a comment. Packages loaded via plugin.Open (core/plugin*, pluginruntime) and cmd/ binaries are excluded.' -Note ("{0} packages scanned, {1} distinct import paths referenced." -f $packages.Count, $imported.Count)

# Test-SanityEnumSibling reports whether the declaration at $Index sits in a
# `const ( ... )` block another member of which is referenced elsewhere.
function Test-SanityEnumSibling {
  param([string]$Text, [int]$Index, [hashtable]$Tokens)
  $open = $Text.LastIndexOf("const (", $Index)
  if ($open -lt 0) { return $false }
  $close = $Text.IndexOf("`n)", $open)
  if ($close -lt 0 -or $close -lt $Index) { return $false }
  $block = $Text.Substring($open, $close - $open)
  foreach ($m in [regex]::Matches($block, '(?m)^\s*([A-Z]\w*)\s*=')) {
    $sibling = $m.Groups[1].Value
    if ($Tokens.ContainsKey($sibling) -and $Tokens[$sibling] -gt 1) { return $true }
  }
  return $false
}

# 2. Exported identifiers with no references
$tokens = Get-SanityTokenCounts -Files $importerFiles
$declRx = New-Object System.Text.RegularExpressions.Regex('(?m)^func\s+(?:\([^)]*\)\s*)?([A-Z]\w*)\s*[(\[]|^type\s+([A-Z]\w*)\b|^\s*([A-Z]\w*)\s*=\s*.*$|^var\s+([A-Z]\w*)\b|^const\s+([A-Z]\w*)\b')
$interfaceNames = @('String', 'Error', 'ServeHTTP', 'MarshalJSON', 'UnmarshalJSON', 'MarshalYAML', 'UnmarshalYAML', 'MarshalText', 'UnmarshalText', 'Scan', 'Value', 'Close', 'Read', 'Write', 'Mount', 'Register', 'Init', 'Name', 'Version', 'Run', 'Start', 'Stop', 'Len', 'Less', 'Swap', 'Validate', 'Migrate', 'Handler', 'Handle', 'Unwrap', 'Is', 'As', 'Shutdown', 'Health', 'Describe', 'Collect', 'RoundTrip', 'Seek', 'Flush', 'Hijack', 'WriteHeader', 'Header', 'Do', 'Next', 'Err', 'Reset', 'Lock', 'Unlock', 'RLock', 'RUnlock', 'ID', 'Type', 'Kind', 'Manifest', 'Routes', 'Widgets', 'Nodes', 'Migrations', 'I18n', 'Hooks', 'Capabilities', 'Configure', 'Setup', 'Teardown', 'OnInstall', 'OnUninstall', 'OnEnable', 'OnDisable', 'OnUpgrade')
$unref = @()
foreach ($f in $backendProd) {
  if ($f.Rel -match '^src/backend/core/plugin/') { continue }                # plugin contract surface
  $text = Get-SanityText $f.Full
  $seen = New-Object 'System.Collections.Generic.HashSet[string]'
  foreach ($m in $declRx.Matches($text)) {
    $name = ''
    for ($g = 1; $g -le 5; $g++) { if ($m.Groups[$g].Success) { $name = $m.Groups[$g].Value; break } }
    if (-not $name -or $seen.Contains($name)) { continue }
    $seen.Add($name) | Out-Null
    if ($interfaceNames -contains $name) { continue }
    if ($name -match '^(Handle|Test|Benchmark|Example)') { continue }
    $count = 0; if ($tokens.ContainsKey($name)) { $count = $tokens[$name] }
    if ($count -gt 1) { continue }
    # A const block is one enum, and the enum is the unit. `ConsoleKindSerial`
    # with no caller today is the protocol's complete value set written down;
    # deleting the members nothing happens to reach yet leaves a partial enum,
    # which is worse than an unused constant. Accepted when a sibling in the
    # same block is referenced.
    if (Test-SanityEnumSibling -Text $text -Index $m.Index -Tokens $tokens) { continue }
    $unref += New-SanityHit -File $f.Rel -Line (Get-SanityLineNumber -Text $text -Index $m.Index) -Text ("{0} - declared here, referenced nowhere in Go sources" -f $name)
  }
}
# A line inside a raw string (a PEM key's base64 ending in `=`) is text, not a declaration.
$unref = @(Remove-SanityRawStringHits -Hits $unref -Root $Root)
Add-SanityCheck -Report $report -Id 2 -Title 'Exported identifiers whose declaration is their only occurrence' -Hits $unref -Mode review -Severity low `
  -Remediation 'Delete, unexport, or note the reflection/serialization reason. Counts are token-based across src/backend, src/agent and src/plugins; identifiers used only from _test.go files are excluded from the count.' -Note 'Handle*/Test* names, common interface methods and core/plugin contract symbols are skipped.'

# 3. Modules mounted but never wired in main.go / core/router
$wiredText = ''
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend/main.go', 'src/backend/core/router') -Include @('*.go'))) { $wiredText += "`n" + (Get-SanityText $f.Full) }
$aliasMap = @{}
foreach ($m in [regex]::Matches($wiredText, '(?m)^\s*(?:(\w+)\s+)?"' + [regex]::Escape($goModule) + '/modules/(\w+)"')) {
  $mod = $m.Groups[2].Value
  $alias = if ($m.Groups[1].Success -and $m.Groups[1].Value) { $m.Groups[1].Value } else { $mod }
  $aliasMap[$mod] = $alias
}
$notWired = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend/modules') -Include @('routes.go'))) {
  if ((Get-SanityText $f.Full) -notmatch '(?m)^func Mount\(') { continue }
  $mod = ($f.Rel -replace '^src/backend/modules/', '') -replace '/.*$', ''
  $pkg = [regex]::Match((Get-SanityText $f.Full), '(?m)^package\s+(\w+)').Groups[1].Value
  $alias = if ($aliasMap.ContainsKey($mod)) { $aliasMap[$mod] } else { $pkg }
  # Mount(app) and its injected variants (MountWithRuntime, MountWithService,
  # MountPlugins, MountWithExecutor, MountWithTriggerService, ...) all count.
  if ($wiredText -notmatch ('\b' + [regex]::Escape($alias) + '\.Mount\w*\(')) { $notWired += New-SanityHit -File $f.Rel -Line 0 -Text ("{0}.Mount*(app) is never called from main.go or core/router (import alias '{1}')" -f $pkg, $alias) }
}
Add-SanityCheck -Report $report -Id 3 -Title 'Modules with Mount() that main.go never wires' -Hits $notWired -Mode strict -Severity high -Remediation 'Call <module>.Mount(app) (or a MountWith* variant) in main.go, or delete the dead module.' -Command 'rg -n "\.Mount\w*\(app" src/backend/main.go'

# 4. Plugins shipped but not registered in the addon index
$indexFile = Join-Path $Root 'src\plugins\index.yaml'
$pluginsDir = Join-Path $Root 'src\plugins\Plugins'
$notIndexed = @(); $noCatalogJson = @()
if ((Test-Path -LiteralPath $indexFile) -and (Test-Path -LiteralPath $pluginsDir)) {
  $indexed = New-Object 'System.Collections.Generic.HashSet[string]'
  foreach ($m in [regex]::Matches((Get-SanityText $indexFile), '(?m)^\s*-?\s*(?:slug|id):\s*"?([\w-]+)"?')) { $indexed.Add($m.Groups[1].Value) | Out-Null }
  foreach ($d in (Get-ChildItem -LiteralPath $pluginsDir -Directory)) {
    if ($d.Name -match '^_') { continue }
    # A plugin is reachable through either delivery path: the signed
    # index.yaml (release tarballs) or src/plugins/catalog/<slug>.json (the
    # local catalog the App Store reads via readLocalCatalogDir). Only a
    # folder in neither is orphaned.
    $hasCatalogJson = Test-Path -LiteralPath (Join-Path $Root "src\plugins\catalog\$($d.Name).json")
    if (-not $indexed.Contains($d.Name) -and -not $hasCatalogJson) { $notIndexed += New-SanityHit -File "src/plugins/Plugins/$($d.Name)" -Line 0 -Text 'not referenced by src/plugins/index.yaml and no src/plugins/catalog/<slug>.json (host will not load it)' }
    if (-not $hasCatalogJson) { $noCatalogJson += New-SanityHit -File "src/plugins/Plugins/$($d.Name)" -Line 0 -Text 'no src/plugins/catalog/<slug>.json' }
  }
  Add-SanityCheck -Report $report -Id 4 -Title 'Plugin folders missing from src/plugins/index.yaml and the local catalog' -Hits $notIndexed -Mode strict -Severity high -Remediation 'Add the plugin to the signed index (make sign-index) or generate src/plugins/catalog/<slug>.json, or remove the folder.' -Note ("{0} slugs in the index." -f $indexed.Count)
  Add-SanityCheck -Report $report -Id '4b' -Title 'Plugin folders without a catalog JSON' -Hits $noCatalogJson -Mode review -Severity low -Remediation 'Generate src/plugins/catalog/<slug>.json so the App Store can list it.'
} else {
  Add-SanityCheck -Report $report -Id 4 -Title 'Plugin folders missing from src/plugins/index.yaml' -Skipped -SkipReason 'src/plugins/index.yaml or src/plugins/Plugins not found'
}

# 5. Ignored errors inside orphaned packages
$orphanDirs = @($orphanPkgs | ForEach-Object { $_.File }) + @($notWired | ForEach-Object { $_.File -replace '/routes\.go$', '' })
$ignoredInOrphans = @()
if ($orphanDirs.Count) {
  $ignoredInOrphans = @(Find-Pattern -Root $Root -Paths $orphanDirs -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern '_, _ =|_, _ :=')
}
Add-SanityCheck -Report $report -Id 5 -Title 'Ignored errors (_, _ =) inside orphaned packages' -Hits $ignoredInOrphans -Mode info -Severity low -Note 'Dead code that also ignores its errors; deleting the package resolves both.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
