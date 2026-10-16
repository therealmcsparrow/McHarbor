# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Code quality sanity check. Mirrors .instructions/sanity/code-quality.md.
.EXAMPLE
  .\scripts\sanity\code-quality\check.ps1
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

$report = New-SanityReport -Name 'code-quality' -Title 'Code Quality Sanity' -Scope 'src/backend, src/frontend, src/agent' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$srcPaths = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend', 'src/frontend', 'src/agent'))
$feScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend'))
$goScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend', 'src/agent'))
$feModScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend/modules'))
$beModCoreAgentScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules', 'src/backend/core', 'src/agent'))
$feModResourcesScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend/modules', 'src/frontend/resources'))
$beModScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules'))
$fe = @{ Root = $Root; Paths = $feScope; Include = @('*.ts', '*.tsx'); ExcludeMatch = @('\.test\.tsx?$', '\.stories\.tsx?$', '\.d\.ts$') }
$go = @{ Root = $Root; Paths = $goScope; Include = @('*.go'); ExcludeMatch = @('_test\.go$') }

# 1. Commented-out code
# Real commented-out code: the keyword is followed by whitespace (so prose
# like "// type-assert via ..." or "// for; the next touch ..." is not a
# match) and the line ends in a code terminator. A line containing sentence
# punctuation (". ") is prose that happens to start with a keyword.
$commented = @(Find-Pattern -Root $Root -Paths $srcPaths -Include @('*.go', '*.ts', '*.tsx') -Pattern '^\s*//\s*(if|for|switch|return|const|let|var|func|type)\s+\S.*[{};,)]\s*$' -NotMatch '\.\s|\bare\b|\bthe\b|\bthat\b')
Add-SanityCheck -Report $report -Id 1 -Title 'Commented-out executable code' -Hits $commented -Mode review -Severity low `
  -Remediation 'Delete or restore; version control already keeps history.' -Command 'rg -n "^\s*//\s*(if|for|switch|return|const|let|var|func|type)\b" src'

# 2. any / ts suppressions
$anyTypes = @(Find-Pattern @fe -StripStrings -Pattern '(:\s*any\b(?![\x27"])|<any>|as any\b|\bany\[\]|Array<any>|Record<[^,>]+,\s*any>|Promise<any>)' -NotMatch '^\s*(//|\*)|(eslint|oxlint)-disable')
$tsIgnore = @(Find-Pattern @fe -Pattern '@ts-ignore')
$tsExpect = @(Find-Pattern @fe -Pattern '@ts-expect-error' -NotMatch '@ts-expect-error\s*[:\-]?\s*\S+')
Add-SanityCheck -Report $report -Id '2a' -Title 'Explicit any types' -Hits $anyTypes -Mode strict -Severity medium -Remediation 'Type it (unknown + narrowing, generics, or a proper interface).' -Command 'rg -n "\bany\b" src/frontend'
Add-SanityCheck -Report $report -Id '2b' -Title '@ts-ignore suppressions' -Hits $tsIgnore -Mode strict -Severity medium -Remediation 'Use @ts-expect-error with a justification, or fix the type.'
Add-SanityCheck -Report $report -Id '2c' -Title '@ts-expect-error without justification' -Hits $tsExpect -Mode strict -Severity low -Remediation 'Append the reason after the directive.'

# 3. console.log
$consoleLog = @(Find-Pattern @fe -Pattern 'console\.log\(' -NotMatch '^\s*//')
Add-SanityCheck -Report $report -Id 3 -Title 'console.log in frontend application code' -Hits $consoleLog -Mode strict -Severity low -Remediation 'Remove, or route through a logger gated on import.meta.env.DEV.' -Command 'rg -n "console\.log\(" src/frontend'

# 4. Ignored Go errors
$ignored = @(Find-Pattern @go -Pattern '(?<![\w,]\s*)_, _ (=|:=)')
Add-SanityCheck -Report $report -Id 4 -Title 'Ignored Go errors (_, _ =)' -Hits $ignored -Mode strict -Severity medium -Remediation 'Log or propagate the error; never drop both return values.' -Command 'rg -n "_, _ =" src/backend src/agent'

# 5. Empty frontend catches
$emptyCatch = @(Find-Pattern @fe -Multiline -Pattern 'catch\s*(\([^)]*\))?\s*\{\s*\}')
$emptyCatchArrow = @(Find-Pattern @fe -Multiline -Pattern '\.catch\(\s*\([^)]*\)\s*=>\s*\{\s*\}\s*\)')
Add-SanityCheck -Report $report -Id 5 -Title 'Empty catch blocks' -Hits ($emptyCatch + $emptyCatchArrow) -Mode strict -Severity medium `
  -Remediation 'Handle or at least comment the intent inside the block (an inline comment is enough to pass).' -Command 'rg -n "catch\s*\([^)]*\)\s*\{\s*\}" src/frontend'

# 6. TODO vs Stub
$todo = @(Find-Pattern -Root $Root -Paths $srcPaths -Include @('*.go', '*.ts', '*.tsx') -Pattern '\bTODO:')
Add-SanityCheck -Report $report -Id 6 -Title 'TODO: comments (repo convention is Stub:)' -Hits $todo -Mode strict -Severity low -Remediation 'Rename to // Stub: or resolve the item.' -Command 'rg -n "TODO:" src'

# 7. panic()
$panics = @(Find-Pattern -Root $Root -Paths $beModCoreAgentScope -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern '\bpanic\(' -NotMatch '^\s*//|recover\(\)|panic\(rec\)|//\s*panic-ok:\s*\S')
Add-SanityCheck -Report $report -Id 7 -Title 'panic() in handler, service or core code' -Hits $panics -Mode strict -Severity high -Remediation 'Return an error; only main.go start-up may abort the process.' -Command 'rg -n "\bpanic\(" src/backend/modules src/backend/core src/agent'

# 8. Magic-number timeouts
$timeoutHits = @(Find-Pattern @go -Pattern '\b(\d+)\s*\*\s*time\.(Millisecond|Second|Minute|Hour)\b' -NotMatch '^\s*(const|\w+\s*=\s*\d+\s*\*\s*time\.)|^\s*\w+\s+=\s*\d+\s*\*|Timeout\s*=|= \d+ \* time')
$groups = @{}
# Demo fixture generators use these literals as synthetic timestamp offsets
# (a run started 2 minutes ago, an incident 24 hours old), not as deadlines.
$timeoutHits = @($timeoutHits | Where-Object { $_.File -notmatch '(^|/)demo/' })
foreach ($h in $timeoutHits) {
  $m = [regex]::Match($h.Text, '(\d+)\s*\*\s*time\.(Millisecond|Second|Minute|Hour)')
  if (-not $m.Success) { continue }
  # Group by role as well as value. A deadline, a ticker interval and an HTTP
  # client budget that all happen to be 30s are three different numbers that
  # look alike; only the first kind is one concept worth naming once.
  $role = 'other'
  if ($h.Text -match 'context\.With(Timeout|Deadline)\(') { $role = 'deadline' }
  elseif ($h.Text -match 'NewTicker\(|NewTimer\(|time\.After\(|time\.Tick\(') { $role = 'interval' }
  elseif ($h.Text -match '\w*Timeout:\s*$|\w*Timeout:\s') { $role = 'client-timeout' }
  $k = "$($m.Groups[1].Value) * time.$($m.Groups[2].Value) [$role]"
  if (-not $groups.ContainsKey($k)) { $groups[$k] = New-Object System.Collections.Generic.List[object] }
  $groups[$k].Add($h)
}
# Repetition only means something inside one package. `10 * time.Second`
# appearing in twenty-eight unrelated files is twenty-eight different
# deadlines that happen to share a number; folding them into one constant
# would couple a Docker probe to a scan poll to an agent handshake, and the
# next person to tune one would silently retune all three. Repetition inside
# a single package is the case where the number really is one concept.
$repeated = @()
foreach ($k in ($groups.Keys | Sort-Object)) {
  $byPkg = @{}
  foreach ($h in $groups[$k]) {
    $pkg = ($h.File -replace '/[^/]+$', '')
    if (-not $byPkg.ContainsKey($pkg)) { $byPkg[$pkg] = New-Object System.Collections.Generic.List[object] }
    $byPkg[$pkg].Add($h)
  }
  foreach ($pkg in ($byPkg.Keys | Sort-Object)) {
    $files = @($byPkg[$pkg] | ForEach-Object { $_.File } | Sort-Object -Unique)
    if ($files.Count -ge 3) { $repeated += "``$k`` literal used in $($files.Count) files of $pkg ($($byPkg[$pkg].Count) places), e.g. $($files[0]), $($files[1])" }
  }
}
Add-SanityCheck -Report $report -Id 8 -Title 'Timeout literal repeated across files in one package' -Hits $repeated -Mode review -Severity low `
  -Remediation 'Promote to a named constant in that package (e.g. dockerCallTimeout). Across packages the same number is usually a coincidence, not a shared concept.' -Command 'rg -n "\d+\s*\*\s*time\.(Millisecond|Second|Minute)" src/backend src/agent'

# 9. Duplicate code hotspots (inventory)
$queryKeys = @(Find-Pattern -Root $Root -Paths $feModResourcesScope -Include @('*.ts', '*.tsx') -Pattern 'queryKey:\s*\[')
$respCodes = @(Find-Pattern -Root $Root -Paths $beModScope -Include @('*.go') -Pattern 'response\.(BadRequest|InternalError|Unauthorized|NotFound|Conflict|Forbidden)Code')
$qkByFile = @($queryKeys | Group-Object File | Where-Object { $_.Count -ge 8 } | Sort-Object Count -Descending | ForEach-Object { "$($_.Name): $($_.Count) inline queryKey arrays" })
$rcByFile = @($respCodes | Group-Object File | Where-Object { $_.Count -ge 40 } | Sort-Object Count -Descending | ForEach-Object { "$($_.Name): $($_.Count) response.*Code calls" })
Add-SanityCheck -Report $report -Id 9 -Title 'Duplicate-code hotspots (files dense with repeated helpers)' -Hits ($qkByFile + $rcByFile) -Mode info -Severity low `
  -Note ("Totals: {0} queryKey arrays, {1} response.*Code calls. Files listed exceed 8 queryKeys or 40 response calls; consider a shared query-key factory or handler helper." -f $queryKeys.Count, $respCodes.Count)

# 10. Copyright header drift (spot-check; full sweep lives in scripts/sanity/copyright)
$spot = @()
$viteEnv = Join-Path $Root 'src\frontend\vite-env.d.ts'
if (Test-Path -LiteralPath $viteEnv) {
  $head = Get-SanityLines $viteEnv
  if ($head.Length -lt 2 -or $head[0] -ne '// Copyright (c) 2026 McSparrow. All rights reserved.') { $spot += New-SanityHit -File 'src/frontend/vite-env.d.ts' -Line 1 -Text 'header missing or wrong' }
}
Add-SanityCheck -Report $report -Id 10 -Title 'Copyright header spot-check (vite-env.d.ts)' -Hits $spot -Mode strict -Severity low -Note 'Run scripts/sanity/copyright for the full sweep.' -Command 'Get-Content src/frontend/vite-env.d.ts -TotalCount 3'

# 11. Page layout contract
#
# This used to ask every page to wrap itself in <PageLayout>. That component
# was a leftover: AppLayout already renders <PageShell>, which owns the page's
# padding and is the one scroller on the screen (see PageShell.tsx). Wrapping
# a page in PageLayout nested a second `overflow-auto` inside it and doubled
# the `p-6` — the exact fault PageShell's own dev-time single-scroller
# assertion exists to catch. PageLayout is gone; what a page owes the shell is
# checked directly instead.
$pageScroller = @(Find-Pattern -Root $Root -Paths $feModScope -Include @('*Page.tsx') -Multiline `
    -Pattern 'return \(\s*<(div|Box)[^>]*className="[^"]*\boverflow-(auto|y-auto|x-auto)\b' -NotMatch 'data-scroll-exempt|PageOverflow')
# The shell drops its own p-6 while the header slot is claimed, so the one
# page that claims the slot puts the padding back. Any other page re-adding it
# is padding on top of padding.
$pagePadding = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $feModScope -Include @('*Page.tsx'))) {
  $text = Get-SanityText $f.Full
  if ($text -notmatch 'return \(\s*<(div|Box)[^>]*className="[^"]*\bp-6\b') { continue }
  if ($text -match 'useHeaderSlot|HeaderBreadcrumb|HeaderActions') { continue }
  $pagePadding += New-SanityHit -File $f.Rel -Line 0 -Text 'page root re-adds p-6 without claiming the header slot'
}
Add-SanityCheck -Report $report -Id '11a' -Title 'Page root introducing a second scroll container' -Hits $pageScroller -Mode review -Severity medium `
  -Remediation 'PageShell (or its PageBody) is the one scroller. Wide content that owns its overflow goes in <PageOverflow>, or carries data-scroll-exempt.' `
  -Command 'rg -n "overflow-(auto|y-auto)" src/frontend/modules -g "*Page.tsx"'
Add-SanityCheck -Report $report -Id '11b' -Title 'Page root re-adding the shell padding' -Hits $pagePadding -Mode review -Severity medium `
  -Remediation 'AppLayout gives PageShell p-6 unless the page claims the header slot. Drop the p-6, or claim the slot.'

Add-SanityNote -Report $report -Text 'Accepted: documented @ts-expect-error with a reason, obvious one-off operational timeouts in small scopes.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
