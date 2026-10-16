# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Permissions sanity check. Mirrors .instructions/sanity/permissions.md.
.DESCRIPTION
  Verifies the six layers a permission key must exist in (const, AllPermissions,
  CatalogSeed, PERM mirror, route gate, UI gate) plus migration grants and plugin
  key collisions. Inventory counts are recorded in the report; a zero inventory
  means the setup regex broke and is reported as a failure, never a clean run.
.EXAMPLE
  .\scripts\sanity\permissions\check.ps1
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [string[]]$Paths
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }
$routesScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules'))
$migrationsScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/core/db/migrations'))
$pluginsScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/plugins'))
$feWidgetNodeScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend', 'widgets', 'nodes'))
$feScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend'))

$report = New-SanityReport -Name 'permissions' -Title 'Permissions Sanity' -Scope 'core/rbac/permissions.go, core/rbac/catalog.go, frontend permissionKeys.ts, modules/**/routes.go, migrations, src/plugins' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

# ---- Shared setup: four inventories ----
$permGo = Join-Path $Root 'src\backend\core\rbac\permissions.go'
$catalogGo = Join-Path $Root 'src\backend\core\rbac\catalog.go'
$permTsFile = Join-Path $Root 'src\frontend\core\auth\permissionKeys.ts'
$constMap = @{}      # PermX -> "x.y"
$allPerms = @()      # PermX in AllPermissions
$seedMap = @{}       # PermX -> @{cat; act; scope}
$permTs = @{}        # "x.y" -> TS_NAME
if (Test-Path -LiteralPath $permGo) {
  $src = Get-SanityText $permGo
  foreach ($m in [regex]::Matches($src, '(?m)^\s*(?:const\s+)?(Perm\w+)\s+Permission\s+=\s+"([^"]+)"')) { $constMap[$m.Groups[1].Value] = $m.Groups[2].Value }
  $body = [regex]::Match($src, 'var AllPermissions = \[\]Permission\{(?<body>[\s\S]*?)\n\}')
  if ($body.Success) { $allPerms = @([regex]::Matches($body.Groups['body'].Value, 'Perm\w+') | ForEach-Object { $_.Value } | Sort-Object -Unique) }
}
foreach ($seamGo in @(Get-ChildItem -LiteralPath (Split-Path -Parent $permGo) -Filter 'permissions_*.go' -File -ErrorAction SilentlyContinue | Where-Object { $_.Name -notlike '*_test.go' })) {
  foreach ($m in [regex]::Matches((Get-SanityText $seamGo.FullName), '(?m)^\s*(?:const\s+)?(Perm\w+)\s+Permission\s+=\s+"([^"]+)"')) { $constMap[$m.Groups[1].Value] = $m.Groups[2].Value }
}
if (Test-Path -LiteralPath $catalogGo) {
  foreach ($m in [regex]::Matches((Get-SanityText $catalogGo), '\{(Perm\w+),\s*"([^"]+)",\s*"([^"]+)",\s*"[^"]*",\s*"([^"]+)"\}')) { $seedMap[$m.Groups[1].Value] = @{ cat = $m.Groups[2].Value; act = $m.Groups[3].Value; scope = $m.Groups[4].Value } }
}
if (Test-Path -LiteralPath $permTsFile) {
  foreach ($m in [regex]::Matches((Get-SanityText $permTsFile), "(?m)^\s*([A-Z0-9_]+):\s*'([^']+)',")) { $permTs[$m.Groups[2].Value] = $m.Groups[1].Value }
}
$keys = @($constMap.Values | Sort-Object -Unique)
$inventory = "consts=$($constMap.Count) allPerms(local-regex,informational)=$($allPerms.Count) seed=$($seedMap.Count) perm=$($permTs.Count)"
Add-SanityNote -Report $report -Text "Inventory: $inventory"
$setupBroken = @()
if ($constMap.Count -eq 0) { $setupBroken += 'const inventory is 0 - permissions.go regex did not match' }
if ($seedMap.Count -eq 0) { $setupBroken += 'CatalogSeed inventory is 0 - catalog.go regex did not match' }
if ($permTs.Count -eq 0) { $setupBroken += 'PERM inventory is 0 - permissionKeys.ts regex did not match' }
Add-SanityCheck -Report $report -Id 0 -Title "Setup inventories ($inventory)" -Hits $setupBroken -Mode strict -Severity high -Remediation 'Fix the setup regex before trusting any result below.' `
  -Note 'AllPermissions() became a function (allPermissionsBase plus RegisterPermissions() entries), so it can no longer be parsed as a literal slice; that inventory is informational only and no longer gates setup-broken. Layers 1, 3, 4 and 5 below delegate to the Go contract tests instead of re-deriving it.'
if ($setupBroken.Count -gt 0) { Set-SanitySetupBroken -Report $report -Reason ("setup inventory broken: {0}" -f ($setupBroken -join '; ')) }

function Get-SanityDottedKeyLines {
  param([string]$Text)
  if ([string]::IsNullOrEmpty($Text)) { return @() }
  return @([regex]::Matches($Text, '(?m)^\s*([a-z][a-z0-9_]+(?:\.[a-z][a-z0-9_]+)+)\s*$') | ForEach-Object { $_.Groups[1].Value })
}

$delegateTargets = @('TestEveryPermissionIsInTheCatalog', 'TestEveryCatalogEntryIsInAllPermissions', 'TestFrontendMirrorMatchesBackendCatalog')
$goBin = Resolve-SanityTool 'go'
$backendDir = Join-Path $Root 'src\backend'
$delegated = $false
$hardFailure = $false
$buildHit = ''
$testOutput = @{}
$testResult = @{}
$goRunNote = "go test ./core/rbac/ -run 'TestEvery|TestFrontendMirror' did not run (go not found on PATH, or src/backend is missing); layers 1, 3, 4 and 5 are reported as skipped this run, not failed."
if ($goBin -and (Test-Path -LiteralPath $backendDir)) {
  $prevToolchain = $env:GOTOOLCHAIN; $prevWork = $env:GOWORK
  $env:GOTOOLCHAIN = 'auto'; $env:GOWORK = 'off'
  $r = Invoke-SanityCommand -FilePath $goBin -Arguments "test ./core/rbac/ -run `"TestEvery|TestFrontendMirror`" -json" -WorkingDirectory $backendDir -TimeoutSec 180
  if ($null -ne $prevToolchain) { $env:GOTOOLCHAIN = $prevToolchain } else { Remove-Item Env:\GOTOOLCHAIN -ErrorAction SilentlyContinue }
  if ($null -ne $prevWork) { $env:GOWORK = $prevWork } else { Remove-Item Env:\GOWORK -ErrorAction SilentlyContinue }
  if (-not [string]::IsNullOrWhiteSpace($r.StdOut)) {
    $packageFailAction = $false
    $packageOutput = ''
    foreach ($line in ($r.StdOut -split "`r?`n")) {
      if ([string]::IsNullOrWhiteSpace($line)) { continue }
      try { $rec = ConvertFrom-SanityJson $line } catch { continue }
      $test = Get-SanityJsonValue $rec 'Test'
      $action = Get-SanityJsonValue $rec 'Action'
      if (-not $test) {
        if ($action -in @('fail', 'build-fail')) { $packageFailAction = $true }
        if ($action -in @('output', 'build-output')) { $packageOutput += "$(Get-SanityJsonValue $rec 'Output')" }
        continue
      }
      if ($action -eq 'output') {
        if (-not $testOutput.ContainsKey($test)) { $testOutput[$test] = '' }
        $testOutput[$test] += "$(Get-SanityJsonValue $rec 'Output')"
      } elseif ($action -in @('pass', 'fail', 'skip')) {
        $testResult[$test] = $action
      }
    }
    $anyTargetAttributed = @($delegateTargets | Where-Object { $testResult.ContainsKey($_) }).Count -gt 0
    if ($packageFailAction -or ($r.ExitCode -ne 0 -and -not $anyTargetAttributed)) {
      $hardFailure = $true
      $buildText = if ($packageOutput) { $packageOutput.Trim() } else { ("$($r.StdOut)`n$($r.StdErr)").Trim() }
      $buildHit = "go test ./core/rbac/ -run 'TestEvery|TestFrontendMirror' failed to build or run (exit $($r.ExitCode)): $buildText"
      $goRunNote = "go test ./core/rbac/ -run 'TestEvery|TestFrontendMirror' failed to build or run (exit $($r.ExitCode)); none of the three target tests reported a pass/fail, so layers 1, 3, 4 and 5 are reported as FAILED, not skipped - a build failure hides real permission drift instead of proving there is none."
    } else {
      $delegated = $true
      $goRunNote = "Delegated to ``go test ./core/rbac/ -run 'TestEvery|TestFrontendMirror' -json`` from src/backend (GOTOOLCHAIN=auto GOWORK=off). Exit code $($r.ExitCode)."
    }
  } else {
    $reason = if ($r.TimedOut) { 'timed out' } else { ($r.StdErr -split "`n" | Select-Object -First 1) }
    if ($r.TimedOut -or $r.ExitCode -ne 0) {
      $hardFailure = $true
      $buildHit = "go test ./core/rbac/ -run 'TestEvery|TestFrontendMirror' produced no stdout and exited $($r.ExitCode) ($reason)"
      $goRunNote = "go test ./core/rbac/ -run 'TestEvery|TestFrontendMirror' produced no stdout and exited $($r.ExitCode) ($reason); layers 1, 3, 4 and 5 are reported as FAILED, not skipped."
    } else {
      $goRunNote = "go test ./core/rbac/ produced no output ($reason); layers 1, 3, 4 and 5 are reported as skipped this run, not failed."
    }
  }
}

# 1. Const with no CatalogSeed entry (delegated to go test TestEveryPermissionIsInTheCatalog)
if ($hardFailure) {
  Add-SanityCheck -Report $report -Id 1 -Title 'Const with no CatalogSeed() entry (delegated to go test TestEveryPermissionIsInTheCatalog)' -Hits @($buildHit) -Mode strict -Severity high -Remediation 'Fix the build/test failure in core/rbac. A red or non-building contract test hides real permission drift instead of proving there is none, so it must fail this check rather than pass it.' -Note $goRunNote
} elseif (-not $delegated) {
  Add-SanityCheck -Report $report -Id 1 -Title 'Const with no CatalogSeed() entry (delegated to go test TestEveryPermissionIsInTheCatalog)' -Skipped -SkipReason $goRunNote
} else {
  $c1 = @(Get-SanityDottedKeyLines "$($testOutput['TestEveryPermissionIsInTheCatalog'])" | ForEach-Object { "$_ has no CatalogSeed() entry" })
  Add-SanityCheck -Report $report -Id 1 -Title 'Const with no CatalogSeed() entry (delegated to go test TestEveryPermissionIsInTheCatalog)' -Hits $c1 -Mode strict -Severity high -Remediation 'Add the entry to CatalogSeed() with category, action, description and scope; without it the key has no scope and cannot be granted.' -Note $goRunNote
}

# 2. CatalogSeed entry with no const (no Go-test analogue: Go will not compile a CatalogSeed() entry referencing an undeclared const, so this stays a local setup-regex check rather than a real drift check)
if (Test-SanitySetupBroken $report) {
  Add-SanityCheck -Report $report -Id 2 -Title 'CatalogSeed() entry with no const' -Skipped -SkipReason $report.SanitySetupBrokenReason
} else {
  $c2 = @($seedMap.Keys | Where-Object { -not $constMap.ContainsKey($_) } | Sort-Object | ForEach-Object { "$_ in CatalogSeed() but not declared in permissions.go" })
  Add-SanityCheck -Report $report -Id 2 -Title 'CatalogSeed() entry with no const' -Hits $c2 -Mode strict -Severity high -Note 'Normally impossible (Go would not compile); a hit almost always means the setup regex is wrong. Not delegated: no Go test asserts this direction.'
}

# 3. Const missing from AllPermissions (delegated to go test TestEveryCatalogEntryIsInAllPermissions)
if ($hardFailure) {
  Add-SanityCheck -Report $report -Id 3 -Title 'Const missing from AllPermissions (delegated to go test TestEveryCatalogEntryIsInAllPermissions)' -Hits @($buildHit) -Mode strict -Severity high -Remediation 'Fix the build/test failure in core/rbac.' -Note $goRunNote
} elseif (-not $delegated) {
  Add-SanityCheck -Report $report -Id 3 -Title 'Const missing from AllPermissions (delegated to go test TestEveryCatalogEntryIsInAllPermissions)' -Skipped -SkipReason $goRunNote
} else {
  $c3 = @(Get-SanityDottedKeyLines "$($testOutput['TestEveryCatalogEntryIsInAllPermissions'])" | ForEach-Object { "$_ is in CatalogSeed() but missing from AllPermissions()" })
  Add-SanityCheck -Report $report -Id 3 -Title 'Const missing from AllPermissions (delegated to go test TestEveryCatalogEntryIsInAllPermissions)' -Hits $c3 -Mode strict -Severity medium -Remediation 'Append to allPermissionsBase, or register it through RegisterPermissions().' -Note $goRunNote
}

# 4/5. permissionKeys.ts <-> backend agreement (delegated to go test TestFrontendMirrorMatchesBackendCatalog)
if ($hardFailure) {
  Add-SanityCheck -Report $report -Id 4 -Title 'Backend key missing from PERM (delegated to go test TestFrontendMirrorMatchesBackendCatalog)' -Hits @($buildHit) -Mode strict -Severity high -Remediation 'Fix the build/test failure in core/rbac.' -Note $goRunNote
  Add-SanityCheck -Report $report -Id 5 -Title 'PERM key with no backend const (delegated to go test TestFrontendMirrorMatchesBackendCatalog)' -Hits @($buildHit) -Mode strict -Severity high -Remediation 'Fix the build/test failure in core/rbac.' -Note $goRunNote
} elseif (-not $delegated) {
  Add-SanityCheck -Report $report -Id 4 -Title 'Backend key missing from PERM (delegated to go test TestFrontendMirrorMatchesBackendCatalog)' -Skipped -SkipReason $goRunNote
  Add-SanityCheck -Report $report -Id 5 -Title 'PERM key with no backend const (delegated to go test TestFrontendMirrorMatchesBackendCatalog)' -Skipped -SkipReason $goRunNote
} else {
  $fmText = "$($testOutput['TestFrontendMirrorMatchesBackendCatalog'])"
  $splitIdx = $fmText.IndexOf('does not define')
  if ($splitIdx -ge 0) { $fmBefore = $fmText.Substring(0, $splitIdx); $fmAfter = $fmText.Substring($splitIdx) } else { $fmBefore = $fmText; $fmAfter = '' }
  $c4 = @(Get-SanityDottedKeyLines $fmBefore | ForEach-Object { "'$_' has no PERM entry in permissionKeys.ts" })
  $c5 = @(Get-SanityDottedKeyLines $fmAfter | ForEach-Object { "PERM entry '$_' has no backend const (dead UI gate)" })
  Add-SanityCheck -Report $report -Id 4 -Title 'Backend key missing from PERM (delegated to go test TestFrontendMirrorMatchesBackendCatalog)' -Hits $c4 -Mode strict -Severity medium -Remediation 'Add to PERM so UI gates have a constant to reference.' -Note $goRunNote
  Add-SanityCheck -Report $report -Id 5 -Title 'PERM key with no backend const (delegated to go test TestFrontendMirrorMatchesBackendCatalog)' -Hits $c5 -Mode strict -Severity high -Remediation 'Delete the PERM entry, or add the missing backend const if the feature is real.' -Note $goRunNote
}

# 6. Mutating route with no permission middleware
$gate = 'Require(Permission|PermissionV2|GlobalPermission|AnyPermission|AnyGlobalPermission|StackPermission|AnyStackPermission|OwnerOrPermission|AccountPermission|SelfService)\b'
$mut = '(?:\.|^\s*)(Post|Put|Patch|Delete)\('
$tot = 0; $gated = 0; $ungated = @()
$exemptRx = '^\s*//.*(pre-auth|self-service|self|public|unauthenticated|signature|one-shot|bearer|AuditExempt|exempt|no permission)'
$exemptPrefixes = @()
$exemptPrefixFile = Join-Path $Root 'src/backend/api/route_exempt_prefix_test.go'
if (Test-Path -LiteralPath $exemptPrefixFile) {
  $pm = [regex]::Match((Get-SanityText $exemptPrefixFile), '(?s)var exemptPrefixes = \[\]string\{(.*?)\}')
  if ($pm.Success) { $exemptPrefixes = @([regex]::Matches($pm.Groups[1].Value, '"([^"]+)"') | ForEach-Object { $_.Groups[1].Value }) }
}
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $routesScope -Include @('routes.go'))) {
  $lines = Get-SanityLines $f.Full
  $pkgConsts = @{}
  foreach ($gf in @(Get-ChildItem -LiteralPath (Split-Path -Parent $f.Full) -Filter '*.go' -File | Where-Object { $_.Name -notlike '*_test.go' })) {
    foreach ($cm in [regex]::Matches((Get-SanityText $gf.FullName), '(?m)^\s*(?:const\s+)?(\w+)\s*=\s*"((?:[^"\\]|\\.)*)"')) { $pkgConsts[$cm.Groups[1].Value] = $cm.Groups[2].Value }
  }
  # Local variables holding a gate (manage := rbac.RequirePermission(...)) count as gates when used in r.With(manage)
  $gateVars = @([regex]::Matches((Get-SanityText $f.Full), '(?m)^\s*(\w+)\s*:?=\s*rbac\.' + $gate) | ForEach-Object { $_.Groups[1].Value } | Sort-Object -Unique)
  $fileGate = $gate
  if ($gateVars.Count) { $fileGate = $gate + '|\.With\((' + (($gateVars | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')\b' }
  $depth = 0; $covered = @()
  for ($i = 0; $i -lt $lines.Length; $i++) {
    $line = $lines[$i]
    $probe = Remove-SanityStringLiterals $line
    if ($line -match 'r\.Use\(' -and $line -match $fileGate) { $covered += $depth }
    if ($line -match $mut -and $line -notmatch '^\s*//') {
      $tot++
      $isGated = ($line -match $fileGate) -or (@($covered | Where-Object { $_ -le $depth }).Count -gt 0)
      if (-not $isGated -and $i -gt 0 -and $lines[$i - 1] -match $fileGate) { $isGated = $true }   # r.With(...). on the previous line
      if ($isGated) { $gated++ }
      else {
        # The rationale is rarely the *last* comment line above the route — a
        # multi-line exemption note ends on a wrapped sentence. Look at the
        # whole comment block and keep the line that actually names an
        # accepted exemption, so 6b separates "verified" from "unverified"
        # instead of listing every commented route for a human to re-read.
        # Walk back over the contiguous run of route lines and the comment
        # block above them: one rationale usually covers several routes
        # registered together, and it is the first line of that block.
        $comment = ''
        $rationale = ''
        for ($k = $i - 1; $k -ge [Math]::Max(0, $i - 14); $k--) {
          $prev = $lines[$k]
          if ($prev -match '^\s*//') {
            if (-not $comment) { $comment = $prev.Trim() }
            if ($prev -match $exemptRx) { $rationale = $prev.Trim() }
            continue
          }
          # Sibling routes and the group scaffolding they sit in (Register*,
          # r.Route, a bare brace) stand between a route and the comment that
          # explains it; keep walking past those, stop at anything else.
          if (-not $comment -and $prev -match '^\s*(r\.(Get|Post|Put|Patch|Delete|With)\b|app\.Register\w+\(|r\.Route\(|\}\)?,?\s*$|\{\s*$)') { continue }
          break
        }
        if ($rationale) { $comment = $rationale }
        $stmt = $line
        $pd = ([regex]::Matches($probe, '\(')).Count - ([regex]::Matches($probe, '\)')).Count
        for ($j = $i + 1; $j -lt [Math]::Min($lines.Length, $i + 12) -and $pd -gt 0; $j++) {
          $stmt += "`n" + $lines[$j]
          $p2 = Remove-SanityStringLiterals $lines[$j]
          $pd += ([regex]::Matches($p2, '\(')).Count - ([regex]::Matches($p2, '\)')).Count
        }
        $em = [regex]::Match($stmt, 'apispec\.Exempt\(\s*(?:"((?:[^"\\]|\\.)*)"|(\w+)\s*\))')
        if ($em.Success) {
          $exemptReason = if ($em.Groups[1].Success) { $em.Groups[1].Value } else { "$($pkgConsts[$em.Groups[2].Value])" }
          $comment = "apispec.Exempt: $exemptReason"
          $rationale = ''
          if (@($exemptPrefixes | Where-Object { $exemptReason.StartsWith($_) }).Count -gt 0) { $rationale = $comment }
        }
        $hit = New-SanityHit -File $f.Rel -Line ($i + 1) -Text ($(if ($rationale) { '[exemption: named]   ' } else { '' }) + $(if ($comment) { "[comment: $comment]   " } else { '' }) + $line.Trim())
        $hit | Add-Member -NotePropertyName HasComment -NotePropertyValue ([bool]$comment)
        $hit | Add-Member -NotePropertyName Named -NotePropertyValue ([bool]$rationale)
        $ungated += $hit
      }
    }
    $depth += ([regex]::Matches($probe, '\{')).Count - ([regex]::Matches($probe, '\}')).Count
    $covered = @($covered | Where-Object { $_ -le $depth })
  }
}
$ungatedNoComment = @($ungated | Where-Object { -not $_.HasComment })
$ungatedWithComment = @($ungated | Where-Object { $_.HasComment -and -not $_.Named })
$ungatedExempt = @($ungated | Where-Object { $_.Named })
Add-SanityCheck -Report $report -Id '6a' -Title "Mutating routes with no Require*Permission and no explanatory comment (mutating=$tot gated=$gated)" -Hits $ungatedNoComment -Mode strict -Severity high `
  -Remediation 'Add r.With(rbac.RequirePermission(app.RBACService, rbac.PermX)) in routes.go. Accepted exemptions (login/logout/setup, password forgot/reset, self-service profile/MFA/sessions/notifications, SAML ACS, gitops webhook, agent archive transfer) must carry a comment in routes.go.'
Add-SanityCheck -Report $report -Id '6b' -Title 'Mutating routes ungated with a comment that names no accepted exemption' -Hits $ungatedWithComment -Mode review -Severity high -Note 'Compare each against the exemption list in .instructions/sanity/permissions.md check 6, then say which one applies in the comment.'
Add-SanityCheck -Report $report -Id '6c' -Title 'Ungated routes whose comment names an accepted exemption (inventory)' -Hits $ungatedExempt -Mode info -Severity info -Note 'Re-read these when the exemption list changes, not every run.'

# 7. Permission const referenced nowhere outside its declaration
$usedConsts = New-Object 'System.Collections.Generic.HashSet[string]'
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend') -Include @('*.go') -ExcludeMatch @('core/rbac/permissions\.go$', 'core/rbac/catalog\.go$', '_test\.go$'))) {
  foreach ($m in [regex]::Matches((Get-SanityText $f.Full), '\b(Perm\w+)\b')) { $usedConsts.Add($m.Groups[1].Value) | Out-Null }
}
$c7 = @($constMap.Keys | Where-Object { -not $usedConsts.Contains($_) -and $_ -ne 'PermWildcard' } | Sort-Object | ForEach-Object { "$_ ($($constMap[$_])) is declared and catalogued but enforced nowhere" })
if (Test-SanitySetupBroken $report) {
  Add-SanityCheck -Report $report -Id 7 -Title 'Decorative permission consts (never referenced outside rbac)' -Skipped -SkipReason $report.SanitySetupBrokenReason
} else {
  Add-SanityCheck -Report $report -Id 7 -Title 'Decorative permission consts (never referenced outside rbac)' -Hits $c7 -Mode strict -Severity medium -Remediation 'Enforce it on a route or in a handler, or delete it from all five layers.'
}

# 8. Enforced by a route but no UI gate references it
# The route paths a key gates are read from routes.go directly, with r.Route
# prefixes applied. Guessing the path from the key's category was wrong whenever
# the two differ, and they differ often: host.view gates /metrics/*, not /host;
# restore.view gates /container-backups/runs/{runId}/restore-*, not /restore;
# registry.view gates /registries, not /registry. All three read as "no UI at
# all" while their screens were shipped and calling the routes.
$enforced = New-Object 'System.Collections.Generic.HashSet[string]'
$gatedRoutes = @{}   # const -> list of @{Verb; Path}
$permVerbRx = New-Object System.Text.RegularExpressions.Regex('\.(Get|Post|Put|Patch|Delete|Head|Options|Handle|HandleFunc)\(\s*"(/[^"]*)"')
$permRouteRx = New-Object System.Text.RegularExpressions.Regex('\.Route\(\s*"([^"]*)"')
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $routesScope -Include @('routes.go'))) {
  foreach ($m in [regex]::Matches((Get-SanityText $f.Full), 'rbac\.(Perm\w+)')) { $enforced.Add($m.Groups[1].Value) | Out-Null }
  $lines = Get-SanityLines $f.Full
  $depth = 0; $stack = @()
  for ($i = 0; $i -lt $lines.Length; $i++) {
    $line = $lines[$i]
    if ($line -match '^\s*//') { continue }
    $stripped = Remove-SanityStringLiterals $line
    $prefix = ($stack | ForEach-Object { $_.Prefix }) -join ''
    $rm = $permRouteRx.Match($line)
    if ($rm.Success) {
      $o = ([regex]::Matches($stripped, '\{')).Count; $cl = ([regex]::Matches($stripped, '\}')).Count
      $stack += @{ Depth = $depth + $o - $cl; Prefix = $rm.Groups[1].Value }
    }
    $consts = @([regex]::Matches($line, 'rbac\.(Perm\w+)') | ForEach-Object { $_.Groups[1].Value } | Sort-Object -Unique)
    if ($consts.Count -gt 0) {
      foreach ($vm in $permVerbRx.Matches($line)) {
        $verb = $vm.Groups[1].Value.ToUpperInvariant(); if ($verb -in @('HANDLE', 'HANDLEFUNC')) { $verb = 'ANY' }
        $rp = (($prefix + $vm.Groups[2].Value) -replace '/+', '/')
        if ($rp.Length -gt 1) { $rp = $rp.TrimEnd('/') }
        foreach ($c in $consts) {
          if (-not $gatedRoutes.ContainsKey($c)) { $gatedRoutes[$c] = New-Object System.Collections.Generic.List[object] }
          $gatedRoutes[$c].Add([pscustomobject]@{ Verb = $verb; Path = $rp })
        }
      }
    }
    $depth += ([regex]::Matches($stripped, '\{')).Count - ([regex]::Matches($stripped, '\}')).Count
    $stack = @($stack | Where-Object { $_.Depth -le $depth })
  }
}
# Does the frontend call this exact route? {param} and /* become "anything that
# is not a quote", so a chi pattern matches the template literal that calls it.
function Test-SanityFeCallsRoute([string]$text, [string]$path, [string]$verb) {
  $rx = [regex]::Escape($path)
  $rx = $rx -replace '\\\{[^}]*\}', "[^'`"``/]+"
  $rx = $rx -replace '/\\\*', "(/[^'`"``]*)?"
  $tail = "(['`"``?]|/(?=['`"``]|\$\{))"
  if ($verb -in @('POST', 'PUT', 'PATCH', 'DELETE')) {
    return [bool]($text -match ("\.\s*(post|put|patch|del|delete)\b[^(]*\(\s*['`"``](/api)?" + $rx + $tail))
  }
  return [bool]($text -match ("['`"``](/api)?" + $rx + $tail))
}
$uiKeys = New-Object 'System.Collections.Generic.HashSet[string]'
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/frontend', 'widgets', 'nodes') -Include @('*.ts', '*.tsx') -ExcludeMatch @('permissionKeys\.ts$'))) {
  foreach ($m in [regex]::Matches((Get-SanityText $f.Full), 'PERM\.([A-Z0-9_]+)')) { $uiKeys.Add($m.Groups[1].Value) | Out-Null }
}
# Two very different situations were being reported as one. A key gating a
# control that exists is a governance gap: the user meets a 403 toast where a
# disabled button with a tooltip belonged. A key gating a capability the UI
# never reaches at all is a half-built feature — there is no control to gate,
# and no amount of frontend work closes it until someone builds the screen.
$feApiText = ''
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/frontend', 'widgets', 'nodes') -Include @('*.ts', '*.tsx') -ExcludeMatch @('\.test\.tsx?$'))) {
  $feApiText += "`n" + (Get-SanityText $f.Full)
}
$c8 = @(); $c8NoUI = @()
foreach ($c in ($enforced | Sort-Object)) {
  $k = $constMap[$c]; if (-not $k) { continue }
  $tsName = $permTs[$k]
  if (-not $tsName -or $uiKeys.Contains($tsName)) { continue }
  # Reachable when the frontend calls any one of the routes this key actually
  # gates. A module can be half-wired — the list read exists, the CRUD screen
  # does not — so a mutating route counts only against a mutating call.
  $called = $false
  foreach ($r in @($gatedRoutes[$c])) {
    if ($null -eq $r) { continue }
    if (Test-SanityFeCallsRoute -text $feApiText -path $r.Path -verb $r.Verb) { $called = $true; break }
  }
  if ($called) {
    $c8 += "$c ('$k') is enforced by a route the frontend calls, but PERM.$tsName gates no control"
  }
  else {
    $c8NoUI += "$c ('$k') is enforced by a route no frontend code calls - the capability has no UI yet"
  }
}
if (Test-SanitySetupBroken $report) {
  Add-SanityCheck -Report $report -Id 8 -Title 'Route-enforced keys with no UI gate on a control that exists' -Skipped -SkipReason $report.SanitySetupBrokenReason
  Add-SanityCheck -Report $report -Id '8b' -Title 'Route-enforced keys whose capability has no UI at all' -Skipped -SkipReason $report.SanitySetupBrokenReason
} else {
  Add-SanityCheck -Report $report -Id 8 -Title 'Route-enforced keys with no UI gate on a control that exists' -Hits $c8 -Mode review -Severity low -Remediation 'Pass permission={PERM.X} on the control that calls the route so users see a disabled state instead of a 403 toast.'
  Add-SanityCheck -Report $report -Id '8b' -Title 'Route-enforced keys whose capability has no UI at all' -Hits $c8NoUI -Mode review -Severity low `
    -Remediation 'Build the screen, or drop the routes and the key. Nothing on the frontend reaches these, so there is no control to gate — a UI gate cannot close this one.'
}

# 9. Bare permission string literals in the UI
$c9 = @(Find-Pattern -Root $Root -Paths $feWidgetNodeScope -Include @('*.ts', '*.tsx') -ExcludeMatch @('permissionKeys\.ts$', '\.test\.tsx?$') -Pattern "permission=\{?'[a-z_]+\.[a-z_.]+'|usePermission\('[a-z_]+\.[a-z_.]+'|usePermissionGate\('[a-z_]+\.[a-z_.]+'|permission:\s*'[a-z_]+\.[a-z_.]+'")
Add-SanityCheck -Report $report -Id 9 -Title 'Bare permission string literals in the UI' -Hits $c9 -Mode strict -Severity low -Remediation 'Replace with the PERM constant so renames are caught at compile time.' -Command "rg -n `"permission=\{?'[a-z_]+\.[a-z_.]+'|usePermission\('...`" src/frontend"

# 10. Middleware variant inconsistent per key
$variants = @{}
$globalOnSelf = @{}
$gateUseRx = 'Require(Global)?Permission\(app\.RBACService,\s*rbac\.(Perm\w+)\)'
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $routesScope -Include @('routes.go'))) {
  $lines = Get-SanityLines $f.Full
  $gateVarMap = @{}
  foreach ($m in [regex]::Matches((Get-SanityText $f.Full), '(?m)^\s*(\w+)\s*:?=\s*rbac\.' + $gateUseRx)) {
    $gateVarMap[$m.Groups[1].Value] = @{ Global = ($m.Groups[2].Value -eq 'Global'); Const = $m.Groups[3].Value }
  }
  $depth = 0; $stack = @()
  for ($i = 0; $i -lt $lines.Length; $i++) {
    $line = $lines[$i]
    if ($line -match '^\s*//') { continue }
    $stripped = Remove-SanityStringLiterals $line
    $prefix = ($stack | ForEach-Object { $_.Prefix }) -join ''
    $rm = $permRouteRx.Match($line)
    if ($rm.Success) {
      $o = ([regex]::Matches($stripped, '\{')).Count; $cl = ([regex]::Matches($stripped, '\}')).Count
      $stack += @{ Depth = $depth + $o - $cl; Prefix = $rm.Groups[1].Value }
    }
    $uses = @()
    if ($line -notmatch '^\s*\w+\s*:?=\s*rbac\.') {
      foreach ($m in [regex]::Matches($line, $gateUseRx)) { $uses += @{ Global = ($m.Groups[1].Value -eq 'Global'); Const = $m.Groups[2].Value } }
    }
    foreach ($vm in [regex]::Matches($line, '\.With\((\w+)\)')) {
      if ($gateVarMap.ContainsKey($vm.Groups[1].Value)) { $uses += $gateVarMap[$vm.Groups[1].Value] }
    }
    if ($uses.Count -gt 0) {
      $paths = @($permVerbRx.Matches($line) | ForEach-Object { (($prefix + $_.Groups[2].Value) -replace '/+', '/') })
      $onSelf = ($paths.Count -gt 0) -and (@($paths | Where-Object { $_ -notmatch '(^|/)self(/|$)' }).Count -eq 0)
      foreach ($u in $uses) {
        $c = $u.Const; $g = if ($u.Global) { 'RequireGlobalPermission' } else { 'RequirePermission' }
        if (-not $variants.ContainsKey($c)) { $variants[$c] = New-Object 'System.Collections.Generic.HashSet[string]' }
        $variants[$c].Add($g) | Out-Null
        if ($u.Global) { $globalOnSelf[$c] = ($onSelf -and $globalOnSelf[$c] -ne $false) }
      }
    }
    $depth += ([regex]::Matches($stripped, '\{')).Count - ([regex]::Matches($stripped, '\}')).Count
    $stack = @($stack | Where-Object { $_.Depth -le $depth })
  }
}
$c10 = @($variants.Keys | Where-Object { $variants[$_].Count -gt 1 -and $globalOnSelf[$_] -ne $true } | Sort-Object | ForEach-Object { "$_ (scope $($seedMap[$_].scope)) is gated with both RequirePermission and RequireGlobalPermission outside the /self split" })
Add-SanityCheck -Report $report -Id 10 -Title 'Key gated with both RequirePermission and RequireGlobalPermission' -Hits $c10 -Mode strict -Severity medium -Remediation 'Pick one per key. Prefer RequirePermission unless the operation reconfigures the host; RequireGlobalPermission denies per-environment role assignments.'

# 11. Migration grants a key the code does not declare
# Harvest keys statement by statement: only statements that touch the
# roles / role_presets tables grant anything (audit-action seeds and
# settings rows in the same file are not grants). A statement that strips
# keys (NOT IN (...), NOT LIKE 'prefix.%', DELETE ... WHERE key IN (...),
# json_remove) marks those keys as pruned; a pruned key is not a live grant
# even when an earlier statement in the same file seeded it (068 renames
# container_backups.* to backups.* in place; 077 strips the kubernetes keys).
$grantFiles = @(Get-SanityFiles -Root $Root -Paths $migrationsScope -Include @('*.sql') | Where-Object { (Get-SanityText $_.Full) -match 'UPDATE (roles|role_presets)|INSERT INTO roles' })
$granted = @{}
$pruned = New-Object 'System.Collections.Generic.HashSet[string]'
$prunedPrefixes = @()
foreach ($f in $grantFiles) {
  $sqlText = [regex]::Replace((Get-SanityText $f.Full), '(?m)--[^\n]*', '')
  foreach ($stmt in ($sqlText -split ';')) {
    $isPrune = $stmt -match '(?is)\bNOT\s+IN\s*\(|\bNOT\s+LIKE\b|\bjson_remove\b|\bDELETE\s+FROM\s+permission_catalog\b'
    if ($isPrune) {
      foreach ($m in [regex]::Matches($stmt, '(?is)NOT\s+IN\s*\(([^)]*)\)')) { foreach ($k in [regex]::Matches($m.Groups[1].Value, "'([a-z_]+\.[a-z_.]+)'")) { $pruned.Add($k.Groups[1].Value) | Out-Null } }
      foreach ($m in [regex]::Matches($stmt, "(?is)NOT\s+LIKE\s+'([a-z_]+\.)%'")) { $prunedPrefixes += $m.Groups[1].Value }
      foreach ($m in [regex]::Matches($stmt, '(?is)DELETE\s+FROM\s+permission_catalog\s+WHERE\s+key\s+IN\s*\(([^)]*)\)')) { foreach ($k in [regex]::Matches($m.Groups[1].Value, "'([a-z_]+\.[a-z_.]+)'")) { $pruned.Add($k.Groups[1].Value) | Out-Null } }
      foreach ($m in [regex]::Matches($stmt, "(?is)value\s*(?:<>|!=)\s*'([a-z_]+\.[a-z_.]+)'")) { $pruned.Add($m.Groups[1].Value) | Out-Null }
      continue
    }
    # Test for the table reference outside string literals: an audit-action
    # row whose *description* says "API key roles ... were changed" is prose,
    # not a grant.
    $stmtNoStrings = [regex]::Replace($stmt, "'(?:[^']|'')*'", "''")
    if ($stmtNoStrings -notmatch '(?i)\b(roles|role_presets)\b') { continue }
    foreach ($m in [regex]::Matches($stmt, "'([a-z_]+\.[a-z_.]+)'")) { $k = $m.Groups[1].Value; if (-not $granted.ContainsKey($k)) { $granted[$k] = $f.Rel } }
  }
}
foreach ($k in @($granted.Keys)) {
  if ($pruned.Contains($k)) { $granted.Remove($k); continue }
  foreach ($p in $prunedPrefixes) { if ($k.StartsWith($p)) { $granted.Remove($k); break } }
}
$pluginKeys = New-Object 'System.Collections.Generic.HashSet[string]'
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $pluginsScope -Include @('*.json', 'plugin.yaml') -ExcludeMatch @('/dist/', '/node_modules/'))) {
  foreach ($m in [regex]::Matches((Get-SanityText $f.Full), '(?m)^\s*-?\s*"?key"?\s*:\s*"?([a-z_]+\.[a-z_.]+)"?')) { $pluginKeys.Add($m.Groups[1].Value) | Out-Null }
}
$c11 = @($granted.Keys | Where-Object { $keys -notcontains $_ -and -not $pluginKeys.Contains($_) } | Sort-Object | ForEach-Object { "'$_' granted in $($granted[$_]) but declared by no host const or plugin manifest" })
Add-SanityCheck -Report $report -Id 11 -Title 'Migration grants a key the code does not declare' -Hits $c11 -Mode strict -Severity high -Remediation 'Remove the grant, or restore the const if the feature still exists.' -Note ("{0} migration files write role permissions; plugin-declared keys ({1}) are accepted." -f $grantFiles.Count, $pluginKeys.Count)

# 12. Plugin key collides with a host key
$c12 = @($pluginKeys | Where-Object { $keys -contains $_ } | Sort-Object | ForEach-Object { "plugin key '$_' collides with a host permission (host wins, plugin declaration silently dropped)" })
Add-SanityCheck -Report $report -Id 12 -Title 'Plugin permission key collides with a host key' -Hits $c12 -Mode strict -Severity medium -Remediation 'Prefix the plugin key with its family.'

# 13. Nav / menu permission value not in PERM
$navPerms = @{}
foreach ($h in @(Find-Pattern -Root $Root -Paths $feScope -Include @('*.ts', '*.tsx') -ExcludeMatch @('\.test\.tsx?$') -Pattern "permission:\s*'([a-z_]+\.[a-z_.]+)'")) {
  $k = [regex]::Match($h.Text, "permission:\s*'([a-z_]+\.[a-z_.]+)'").Groups[1].Value
  if (-not $navPerms.ContainsKey($k)) { $navPerms[$k] = "$($h.File):$($h.Line)" }
}
$c13 = @($navPerms.Keys | Where-Object { -not $permTs.ContainsKey($_) } | Sort-Object | ForEach-Object { "'$_' at $($navPerms[$_]) resolves to no PERM key (link hidden for everyone)" })
Add-SanityCheck -Report $report -Id 13 -Title 'Nav / menu permission literal not in PERM' -Hits $c13 -Mode strict -Severity medium -Remediation 'Use an existing PERM constant or add the key through all five layers.'

Add-SanityNote -Report $report -Text 'Severity: 1, 2, 5, 6, 11 high; 3, 4, 10, 12, 13 medium; 7, 8, 9 low. PermWildcard (*) is excluded everywhere.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
