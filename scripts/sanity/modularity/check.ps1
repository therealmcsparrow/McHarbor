# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Modularity sanity check. Mirrors .instructions/sanity/modularity.md.
.EXAMPLE
  .\scripts\sanity\modularity\check.ps1
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [string[]]$Paths,
  [switch]$PruneBaseline
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$report = New-SanityReport -Name 'modularity' -Title 'Modularity Sanity' -Scope 'src/backend, src/frontend' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$goModule = 'github.com/therealmcsparrow/mcharbor'
$goModFile = Join-Path $Root 'src\backend\go.mod'
if (Test-Path -LiteralPath $goModFile) {
  $m = [regex]::Match((Get-SanityText $goModFile), '(?m)^module\s+(\S+)')
  if ($m.Success) { $goModule = $m.Groups[1].Value }
}

$beModScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules'))
$feModScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend/modules'))
$feResourcesCoreScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend/resources', 'src/frontend/core'))

# 1. Backend cross-module imports
$allowedEdges = New-Object 'System.Collections.Generic.HashSet[string]'
$archTest = Join-Path $Root 'src\backend\api\arch_test.go'
if (Test-Path -LiteralPath $archTest) {
  $block = [regex]::Match((Get-SanityText $archTest), '(?s)var allowedEdges = map\[string\]string\{(.*?)\n\}')
  foreach ($m in [regex]::Matches($block.Groups[1].Value, '"([\w*]+)->([\w*]+)"\s*:')) {
    $allowedEdges.Add($m.Groups[1].Value + '->' + $m.Groups[2].Value) | Out-Null
  }
}
$crossGo = @(); $crossGoAllowed = 0
foreach ($h in @(Find-Pattern -Root $Root -Paths $beModScope -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern ('"' + [regex]::Escape($goModule) + '/modules/(\w+)'))) {
  $own = ($h.File -replace '^src/backend/modules/', '') -replace '/.*$', ''
  $target = [regex]::Match($h.Text, [regex]::Escape($goModule) + '/modules/(\w+)').Groups[1].Value
  if ($target -eq $own) { continue }
  if ($allowedEdges.Contains("$own->$target") -or $allowedEdges.Contains("$own->*")) { $crossGoAllowed++ } else { $crossGo += $h }
}
Add-SanityCheck -Report $report -Id 1 -Title 'Backend module importing another module' -Hits $crossGo -Mode strict -Severity medium `
  -Remediation 'Move the shared logic to core/ or inject a consumer-side interface from main.go / internal/bootstrap; a structural nesting goes in allowedEdges in src/backend/api/arch_test.go with its reason.' `
  -Note ("{0} import site(s) covered by allowedEdges in src/backend/api/arch_test.go; _test.go files are skipped, as go list .Imports skips them." -f $crossGoAllowed) `
  -Command "rg -n `"$goModule/modules/`" src/backend/modules -g '!*_test.go'"

# 2. Frontend cross-module imports
$crossFe = @()
foreach ($h in @(Find-Pattern -Root $Root -Paths $feModScope -Include @('*.ts', '*.tsx') -Pattern "from ['`"]@modules/([\w-]+)")) {
  $own = ($h.File -replace '^src/frontend/modules/', '') -replace '/.*$', ''
  $target = [regex]::Match($h.Text, "@modules/([\w-]+)").Groups[1].Value
  if ($target -ne $own) { $crossFe += $h }
}
Add-SanityCheck -Report $report -Id 2 -Title 'Frontend module importing another module' -Hits $crossFe -Mode strict -Severity medium `
  -Remediation 'Lift the shared component/hook into @resources or @core instead of importing across modules.' -Command 'rg -n "@modules/" src/frontend/modules'

# 2b. Shared code reaching back into a feature module
#
# Check 2 only looks inside src/frontend/modules, so it cannot see the worse
# direction: @resources or @core importing @modules. That inverts the
# dependency - shared code then depends on a feature, and the "shared" file
# cannot be used without dragging that module in. Found in the wild on
# 2026-09-11 (MetricHistoryPanel in @resources imported the containers module's
# metrics hook) precisely because nothing looked for it.
$sharedBaselinePath = Join-Path $Root '.instructions/sanity/baselines/shared-to-module-imports.txt'
$sharedBaseline = New-Object 'System.Collections.Generic.HashSet[string]'
if (Test-Path -LiteralPath $sharedBaselinePath) {
  foreach ($l in (Get-SanityLines $sharedBaselinePath)) {
    $t = $l.Trim()
    if (-not $t -or $t.StartsWith('#')) { continue }
    $sharedBaseline.Add($t.ToLowerInvariant()) | Out-Null
  }
}
$sharedToModule = @(); $sharedBaselined = 0
foreach ($h in @(Find-Pattern -Root $Root -Paths $feResourcesCoreScope -Include @('*.ts', '*.tsx') -Pattern "from ['`"]@modules/")) {
  # core/router.tsx and the core/router/ files it was split into are the composition root allowed to import every module (module-boundaries rule 6).
  if ($h.File -ieq 'src/frontend/core/router.tsx' -or $h.File -ilike 'src/frontend/core/router/*') { continue }
  if ($sharedBaseline.Contains($h.File.ToLowerInvariant())) { $sharedBaselined++ } else { $sharedToModule += $h }
}
Add-SanityCheck -Report $report -Id '2b' -Title 'Shared code (@resources / @core) importing a feature module' -Hits $sharedToModule -Mode strict -Severity medium `
  -Remediation 'Move the shared dependency down into @resources/@core, or invert the call so the module passes what the shared component needs in as a prop. Shared code must never depend on a feature module. Only add to .instructions/sanity/baselines/shared-to-module-imports.txt when the untangling is intentionally deferred.' `
  -Note ("{0} legacy file(s) covered by the baseline." -f $sharedBaselined) -Command 'rg -n "@modules/" src/frontend/resources src/frontend/core'

# 3. SQL in handlers
$sqlHandlers = @(Find-Pattern -Root $Root -Paths $beModScope -Include @('handler.go') -Pattern '["`]\s*(SELECT|INSERT|UPDATE|DELETE)\s' -NotMatch '^\s*//')
$dbHandlers = @(Find-Pattern -Root $Root -Paths $beModScope -Include @('handler.go') -Pattern $script:SanityDbCallPattern -NotMatch ('^\s*//|' + $script:SanityDbCallNotMatch))
Add-SanityCheck -Report $report -Id 3 -Title 'SQL or DB calls inside handler.go' -Hits ($sqlHandlers + $dbHandlers) -Mode strict -Severity high `
  -Remediation 'Move data access to service.go; the handler only parses, validates, authorizes, calls the service, audits and responds.' `
  -Command 'rg -n "SELECT |INSERT |UPDATE |DELETE " src/backend/modules -g "handler.go"'

# 4. Hook location
#
# Three shapes are already organised, and reporting them drowned the check:
#
#   * `stores/` — a Zustand store is exported as `useXStore` but it is a store,
#     and `stores/` is where the layout puts it.
#   * A file named `useThing.ts(x)` — the file is the hook module, which is the
#     same organisation `hooks/` gives, one directory up.
#   * `*Columns.tsx` / `*-columns.tsx` — a `useXColumns` factory is the column
#     set of the one table it sits beside, not a reusable hook.
#   * A `PascalCase.tsx` file is a component. A hook exported from it is the
#     component-coupled case the remediation already accepts.
$hooksOutside = @(Find-Pattern -Root $Root -Paths $feModScope -Include @('*.ts', '*.tsx') -Pattern '^export (function|const) use[A-Z]\w*' `
  -ExcludeMatch @('/hooks/', '\.test\.', '/stores/', '/use[A-Z]\w*\.tsx?$', '(?i)columns\.tsx?$', '/[A-Z]\w*\.tsx$'))
# A one-line selector over a store is part of that store's surface, not a hook
# that wandered out of hooks/.
$hooksOutside = @($hooksOutside | Where-Object { $_.Text -notmatch '=>\s*use[A-Z]\w*Store\(' })
Add-SanityCheck -Report $report -Id 4 -Title 'Exported hooks living outside hooks/' -Hits $hooksOutside -Mode review -Severity low `
  -Remediation 'Move broadly reusable hooks to <module>/hooks/. Tiny hooks tightly coupled to one component are accepted.' -Command 'rg -n "^export function use[A-Z]" src/frontend/modules'

# 5. Oversized components (with legacy baseline)
$baselinePath = Join-Path $Root '.instructions\sanity\baselines\oversized-components.txt'
$baseline = New-Object 'System.Collections.Generic.HashSet[string]'
if (Test-Path -LiteralPath $baselinePath) {
  foreach ($l in (Get-SanityLines $baselinePath)) {
    $t = $l.TrimStart([char]0xFEFF).Trim()
    if (-not $t -or $t.StartsWith('#')) { continue }
    $t = ($t -replace '\s*\(.*$', '') -replace '\\', '/'
    $baseline.Add($t.ToLowerInvariant()) | Out-Null
  }
}
# The rule counts code lines only: blanks, imports and comments are free, and a test file is not a component.
function Get-CodeLineCount {
  param([string[]]$Lines)
  $n = 0; $inBlock = $false; $inImport = $false
  foreach ($raw in $Lines) {
    $l = $raw.Trim()
    if ($inBlock) { if ($l -match '\*/\s*$') { $inBlock = $false }; continue }
    if ($inImport) { if ($l -match '\bfrom\s+[''"]') { $inImport = $false }; continue }
    if (-not $l) { continue }
    if ($l.StartsWith('//')) { continue }
    if ($l.StartsWith('/*')) { if ($l -notmatch '\*/\s*$') { $inBlock = $true }; continue }
    if ($l -match '^import\b') {
      if ($l -notmatch '\bfrom\s+[''"]' -and $l -notmatch '^import\s+[''"]') { $inImport = $true }
      continue
    }
    $n++
  }
  return $n
}
$oversized = @(); $baselined = 0; $shrunk = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $feModScope -Include @('*.tsx'))) {
  if ($f.Rel -match '\.(test|spec)\.tsx$') { continue }
  $count = Get-CodeLineCount (Get-SanityLines $f.Full)
  $inBaseline = $baseline.Contains($f.Rel.ToLowerInvariant())
  if ($count -gt 300) {
    if ($inBaseline) { $baselined++ } else { $oversized += New-SanityHit -File $f.Rel -Line 0 -Text "$count lines" }
  } elseif ($inBaseline) {
    $shrunk += New-SanityHit -File $f.Rel -Line 0 -Text "$count lines - now under 300, remove from baseline"
  }
}
Add-SanityCheck -Report $report -Id '5a' -Title 'Components over 300 lines not in the legacy baseline' -Hits $oversized -Mode strict -Severity medium `
  -Remediation 'Split the component or extract hooks. Only add to .instructions/sanity/baselines/oversized-components.txt when a refactor is intentionally deferred and documented.' `
  -Note ("{0} legacy files over 300 lines are covered by the baseline." -f $baselined)

# -PruneBaseline: a baseline only ever shrinks. Rewrites the file without the entries 5b found,
# keeping the BOM, the line-ending convention and every header/comment line byte for byte.
$pruned = 0
if ($PruneBaseline -and $shrunk.Count -gt 0 -and (Test-Path -LiteralPath $baselinePath)) {
  $shrunkKeys = New-Object 'System.Collections.Generic.HashSet[string]'
  foreach ($h in $shrunk) { $shrunkKeys.Add($h.File.ToLowerInvariant()) | Out-Null }
  $bytes = [System.IO.File]::ReadAllBytes($baselinePath)
  $hadBom = ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF)
  $utf8 = New-Object System.Text.UTF8Encoding($hadBom)
  $raw = [System.IO.File]::ReadAllText($baselinePath, $utf8)
  $kept = New-Object System.Text.StringBuilder
  foreach ($piece in [regex]::Split($raw, '(?<=\n)')) {
    if ($piece.Length -eq 0) { continue }
    $t = $piece.Trim()
    if ($t -and -not $t.StartsWith('#')) {
      $key = (($t -replace '\s*\(.*$', '') -replace '\\', '/').ToLowerInvariant()
      if ($shrunkKeys.Contains($key)) { $pruned++; Write-Host ("   pruned from baseline: {0}" -f $t); continue }
    }
    [void]$kept.Append($piece)
  }
  [System.IO.File]::WriteAllText($baselinePath, $kept.ToString(), $utf8)
  Write-Host ("   -PruneBaseline: removed {0} line(s) from oversized-components.txt" -f $pruned)
}
$prunedNote = ''
if ($pruned -gt 0) { $prunedNote = "$pruned baseline line(s) pruned by -PruneBaseline in this run." }
$remainingShrunk = $shrunk
if ($pruned -gt 0) { $remainingShrunk = @() }
Add-SanityCheck -Report $report -Id '5b' -Title 'Baseline entries that are now under 300 lines' -Hits $remainingShrunk -Mode review -Severity low -Remediation 'Delete the line from the baseline so it cannot regress silently, or run check.ps1 -PruneBaseline (run-all.ps1 does).' -Note $prunedNote

# 6. Service layer existence for DB-backed modules
$exemptModules = @('logs', 'events', 'terminal', 'health', 'openapi', 'docker_info', 'clock')
$dbDirs = @(Find-Pattern -Root $Root -Paths $beModScope -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern $script:SanityDbCallPattern -NotMatch $script:SanityDbCallNotMatch | ForEach-Object { $_.File -replace '/[^/]+$', '' } | Sort-Object -Unique)
$noService = @()
foreach ($d in $dbDirs) {
  $mod = ($d -replace '^src/backend/modules/', '') -replace '/.*$', ''
  if ($exemptModules -contains $mod) { continue }
  $modDir = Join-Path $Root ('src/backend/modules/' + $mod)
  # store.go is the accepted data-layer half of service.go (module-boundaries rule 3).
  if (-not (Test-Path -LiteralPath (Join-Path $modDir 'service.go')) -and -not (Test-Path -LiteralPath (Join-Path $modDir 'store.go'))) { $noService += New-SanityHit -File $d -Line 0 -Text 'runs SQL but the module has no service.go' }
}
Add-SanityCheck -Report $report -Id 6 -Title 'DB-accessing modules without service.go' -Hits @($noService | Sort-Object File -Unique) -Mode strict -Severity medium `
  -Remediation 'Create service.go and move the queries there.' -Note ("Exempt thin proxies/trivial modules: {0}." -f ($exemptModules -join ', '))

# 7. Mount function presence
$noMount = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $beModScope -Include @('routes.go'))) {
  # A MountContainerRoutes-style variant nests under a parent module's group (chi cannot mount /containers twice).
  if ((Get-SanityText $f.Full) -notmatch 'func Mount\w*\(') { $noMount += New-SanityHit -File $f.Rel -Line 0 -Text 'routes.go without func Mount(' }
}
Add-SanityCheck -Report $report -Id 7 -Title 'routes.go without a Mount() function' -Hits $noMount -Mode strict -Severity high -Remediation 'Expose func Mount(app *router.AppDeps) so main.go can wire the module.' -Command 'rg -n "func Mount\(" src/backend/modules -g "routes.go"'

# 8. Dependency injection conventions
# AppDeps has to come first; a module that also needs a service, a pool or a
# hub takes it after. Requiring AppDeps to be the *only* parameter reported
# nine handlers that follow the convention exactly.
$badHandler = @(Find-Pattern -Root $Root -Paths $beModScope -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern '^func NewHandler\(' -NotMatch 'NewHandler\(app \*router\.AppDeps[,)]')
# The convention exists so a handler can reach the shared app services. One
# that needs none of them and takes only what it uses is following 8b's
# principle, not breaking 8a's — provided it says so in a `narrow deps:`
# comment naming what it does without.
$badHandler = @($badHandler | Where-Object { (Get-SanityText (Join-Path $Root $_.File)) -notmatch 'narrow deps:' })
$fatService = @(Find-Pattern -Root $Root -Paths $beModScope -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern '^func NewService\([^)]*\*router\.AppDeps')
Add-SanityCheck -Report $report -Id '8a' -Title 'NewHandler not taking *router.AppDeps' -Hits $badHandler -Mode review -Severity low -Remediation 'Signature must be NewHandler(app *router.AppDeps).' -Command 'rg -n "NewHandler\(app \*router\.AppDeps\)" src/backend/modules'
Add-SanityCheck -Report $report -Id '8b' -Title 'NewService taking the whole AppDeps container' -Hits $fatService -Mode review -Severity low -Remediation 'Pass the specific dependency (db, ClientPool, encryption) instead of all of AppDeps.' -Command 'rg -n "func NewService\(" src/backend/modules'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
