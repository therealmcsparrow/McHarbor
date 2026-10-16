# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  React standards sanity check. Mirrors .instructions/sanity/react-standards.md.
.EXAMPLE
  .\scripts\sanity\react-standards\check.ps1
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

$report = New-SanityReport -Name 'react-standards' -Title 'React Standards Sanity' -Scope 'src/frontend, widgets, nodes, src/plugins/Plugins/*/frontend/src' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$defaultPaths = @('src/frontend', 'widgets', 'nodes') + @(Get-SanityPluginPaths -Root $Root -SubPath 'frontend/src')
$paths = (Get-SanityScope -Root $Root -Paths $Paths -Default $defaultPaths)
$tsx = @{ Root = $Root; Paths = $paths; Include = @('*.tsx'); ExcludeMatch = @('\.test\.tsx$', '\.stories\.tsx$') }
$ts = @{ Root = $Root; Paths = $paths; Include = @('*.ts', '*.tsx'); ExcludeMatch = @('\.test\.tsx?$', '\.stories\.tsx?$', '\.d\.ts$') }

# 1. No async useEffect
$asyncEffect = @(Find-Pattern @ts -Pattern 'useEffect\s*\(\s*async')
Add-SanityCheck -Report $report -Id 1 -Title 'async callback passed to useEffect' -Hits $asyncEffect -Mode strict -Severity high -Remediation 'Declare an inner async function and call it; return the cleanup synchronously.' -Command 'rg -n "useEffect\s*\(\s*async" src/frontend'

# 2. Missing cleanup in subscribed effects
$noCleanup = @()
foreach ($f in @(Get-SanityFiles @ts)) {
  $text = Get-SanityText $f.Full
  $subs = @()
  # Closing an EventSource/WebSocket drops its listeners, and a listener
  # registered with { once: true } removes itself, so neither needs an
  # explicit removeEventListener.
  # Only React lifecycle files can leak a listener per mount; a module-level
  # singleton listener (no useEffect in the file) is attached once for the life
  # of the document by design.
  if ($text -match 'useEffect' -and $text -match 'addEventListener\(' -and $text -notmatch 'removeEventListener\(' -and $text -notmatch '\.close\(\)' -and $text -notmatch 'once:\s*true') { $subs += 'addEventListener without removeEventListener' }
  if ($text -match 'setInterval\(' -and $text -notmatch 'clearInterval\(') { $subs += 'setInterval without clearInterval' }
  # A fire-and-forget revoke of an object URL is not a lifecycle timer.
  $timerText = $text -replace 'setTimeout\(\s*\(\)\s*=>\s*URL\.revokeObjectURL\([^)]*\)\s*,\s*\d+\s*\)', ''
  if ($text -match 'useEffect' -and $timerText -match 'setTimeout\(' -and $text -notmatch 'clearTimeout\(') { $subs += 'setTimeout inside a file with effects but no clearTimeout' }
  if ($text -match 'new EventSource\(|new WebSocket\(' -and $text -notmatch '\.close\(\)') { $subs += 'EventSource/WebSocket without close()' }
  if ($subs.Count) { $noCleanup += New-SanityHit -File $f.Rel -Line 0 -Text ($subs -join '; ') }
}
Add-SanityCheck -Report $report -Id 2 -Title 'Subscriptions or timers with no matching cleanup in file' -Hits $noCleanup -Mode review -Severity medium -Remediation 'Return a cleanup from the effect that removes the listener / clears the timer / closes the stream.' -Command 'rg -n "addEventListener|setInterval|setTimeout" src/frontend -g "*.tsx" -g "*.ts"'

# 3. Array index as key
$indexKey = @(Find-Pattern @tsx -Pattern 'key=\{\s*(index|i|idx)\s*\}')
# A template key that also carries a data field - `${svc.Name}-${i}` - is
# stable across reorders; the index is only disambiguating duplicates. The
# finding is a key whose entire content is the index.
#
# Static filler - a skeleton line, a padding cell, a menu separator - has no
# data to key on and never reorders, which is the case the remediation already
# accepts. Those keys name themselves.
$indexKeyTpl = @(Find-Pattern @tsx -Pattern 'key=\{`[^`$]*\$\{\s*(index|i|idx)\s*\}[^`$]*`\}' -NotMatch 'key=\{`(pad|row|line|bar|sep|skeleton|[\w-]*skeleton)[-_]')
Add-SanityCheck -Report $report -Id '3a' -Title 'Array index used directly as React key' -Hits $indexKey -Mode strict -Severity medium -Remediation 'Use a stable id from the data.' -Command 'rg -n "key=\{(index|i)\}" src/frontend'
Add-SanityCheck -Report $report -Id '3b' -Title 'Template keys that embed the array index' -Hits $indexKeyTpl -Mode review -Severity low -Remediation 'Accepted for static padding/placeholder rows; use a data id when the list can reorder.'

# 4. Nested component definitions
$nested = @()
foreach ($f in @(Get-SanityFiles @tsx)) {
  $lines = Get-SanityLines $f.Full
  $inComponent = $false
  for ($i = 0; $i -lt $lines.Length; $i++) {
    $l = $lines[$i]
    if ($l -cmatch '^(export\s+)?(default\s+)?function\s+[A-Z]\w*\s*[(<]|^(export\s+)?const\s+[A-Z]\w*\s*(:[^=]+)?=\s*(React\.)?(memo|forwardRef)?\(?\s*(\([^)]*\)|\w+)\s*(:[^=]*)?=>') { $inComponent = $true; continue }
    if ($l -cmatch '^[})]') { $inComponent = $false; continue }
    if (-not $inComponent) { continue }
    if ($l -cmatch '^\s{2,}(function\s+[A-Z]\w*\s*\(|const\s+[A-Z]\w*\s*(:[^=]+)?=\s*\(\s*(\{[^}]*\}|\w+)?\s*(:[^)]*)?\)\s*(:[^=]*)?=>)' -and $l -notmatch '^\s*//') {
      $nested += New-SanityHit -File $f.Rel -Line ($i + 1) -Text $l.Trim()
    }
  }
}
Add-SanityCheck -Report $report -Id 4 -Title 'Component defined inside another component' -Hits $nested -Mode review -Severity medium -Remediation 'Hoist to module scope; inner components remount on every render. Capitalized render helpers returning non-JSX are false positives.' -Command 'rg -n "function [A-Z][A-Za-z0-9]*\(" src/frontend -g "*.tsx"'

# 5. Suppression comments
$tsIgnore = @(Find-Pattern @ts -Pattern '@ts-ignore')
$tsExpect = @(Find-Pattern @ts -Pattern '@ts-expect-error')
Add-SanityCheck -Report $report -Id '5a' -Title '@ts-ignore' -Hits $tsIgnore -Mode strict -Severity medium -Remediation 'Use @ts-expect-error with a justification.' -Command 'rg -n "@ts-ignore|@ts-expect-error" src/frontend'
Add-SanityCheck -Report $report -Id '5b' -Title '@ts-expect-error (inventory, must carry a reason)' -Hits $tsExpect -Mode info -Severity info

# 6. Non-null assertion abuse
$nonNull = @(Find-Pattern @ts -StripStrings -Pattern '[\w\)\]]!(\.|\[|\)|,|;|\s*$)' -NotMatch '!==?|^\s*//|^\s*\*')
$nonNullTop = @($nonNull | Group-Object File | Sort-Object Count -Descending | Where-Object { $_.Count -ge 3 } | Select-Object -First 30 | ForEach-Object { New-SanityHit -File $_.Name -Line 0 -Text "$($_.Count) non-null assertions" })
Add-SanityCheck -Report $report -Id 6 -Title 'Files with 3+ non-null assertions (!)' -Hits $nonNullTop -Mode review -Severity low -Remediation 'Prefer narrowing, optional chaining or an explicit invariant helper.' -Note ("{0} non-null assertions in total." -f $nonNull.Count) -Command 'rg -n "!\.|!\[|!\)" src/frontend -g "*.ts" -g "*.tsx"'

# 7. Unsafe assertions
# A cast at an untyped boundary — res.json(), Object.fromEntries, indexing
# `window` or a namespace import — is not something a better annotation fixes.
# Those carry an `unavoidable cast:` comment naming the boundary; an
# undocumented one is still a finding.
$unsafe = @(Find-Pattern @ts -StripStrings -Pattern '\bas any\b|as unknown as\b' -NotMatch '^\s*//')
$unsafe = @($unsafe | Where-Object {
    # Playwright specs drive a browser through window globals and sit under eslint's testIgnores, like *.test.ts.
    if ($_.File -match '\.spec\.tsx?$') { return $false }
    $lines = Get-SanityLines (Join-Path $Root ($_.File -replace '/', [System.IO.Path]::DirectorySeparatorChar))
    $documented = $false
    for ($k = [Math]::Max(0, $_.Line - 4); $k -lt $_.Line; $k++) { if ($lines[$k] -match 'unavoidable cast:') { $documented = $true } }
    -not $documented
  })
Add-SanityCheck -Report $report -Id 7 -Title 'as any / as unknown as casts' -Hits $unsafe -Mode review -Severity medium -Remediation 'Fix the type at the source; double casts hide real mismatches.' -Command 'rg -n "as any|as unknown as" src/frontend'

# 8. Click handlers on non-interactive elements
$clickDiv = @(Find-Pattern @tsx -Multiline -Pattern "<(div|span|li|td|tr|p)\b$($script:SanityJsxTagBody)\bonClick=$($script:SanityJsxTagBody)>" -NotMatch 'role=|tabIndex|onKeyDown|onKeyUp|onClick=\{\(?\w*\)?\s*=>\s*\w+\.stopPropagation\(\)\}')
Add-SanityCheck -Report $report -Id 8 -Title 'onClick on non-interactive elements without role/tabIndex/keyboard handler' -Hits $clickDiv -Mode review -Severity medium -Remediation 'Use <Button variant="ghost"> or add role="button", tabIndex={0} and a keyboard handler.' -Command 'rg -n "<div[^>]*onClick=|<span[^>]*onClick=" src/frontend -g "*.tsx"'

# 9. Images without alt
$imgNoAlt = @(Find-Pattern @tsx -Multiline -Pattern "<img\s$($script:SanityJsxTagBody)>" -NotMatch '\balt=|\{\.\.\.')
Add-SanityCheck -Report $report -Id 9 -Title '<img> without alt' -Hits $imgNoAlt -Mode strict -Severity medium -Remediation 'Add alt={t(...)} or alt="" for decorative images.' -Command 'rg -nP "<img(?![^>]*\balt=)" src/frontend -g "*.tsx"'

# 10. Icon-only buttons without aria-label
$iconTags = @(Find-Pattern @tsx -Multiline -Pattern "<(Button|IconButton|HeaderActionButton|ActionButton)\b$($script:SanityJsxTagBody)\bsize=[`"']icon(-sm|-xs|-lg)?[`"']$($script:SanityJsxTagBody)>")
$iconNoLabel = @($iconTags | Where-Object { $_.Match -notmatch 'aria-label' -and $_.Match -notmatch '\{\.\.\.' -and $_.Match -notmatch '\btitle=' })
Add-SanityCheck -Report $report -Id 10 -Title 'size="icon" buttons without aria-label' -Hits $iconNoLabel -Mode strict -Severity medium -Remediation 'aria-label={t("...")} on every icon-only button.' -Command 'rg -n "<Button[^>]*size=\"icon\"" src/frontend -g "*.tsx"'

# 11. Inline style objects
$inline = @(Find-Pattern @tsx -Pattern 'style=\{\{')
Add-SanityCheck -Report $report -Id 11 -Title 'Inline style objects' -Hits $inline -Mode strict -Severity medium -Remediation 'Tailwind utilities / arbitrary values; CSS variables for dynamic values.' -Command 'rg -n "style=\{\{" src/frontend -g "*.tsx"'

# 12. useTranslation without namespace
$bareNs = @(Find-Pattern @ts -Pattern 'useTranslation\(\s*\)' -NotMatch '^\s*(//|\*|/\*)')
Add-SanityCheck -Report $report -Id 12 -Title 'Bare useTranslation()' -Hits $bareNs -Mode strict -Severity medium -Remediation "useTranslation('<namespace>')." -Command 'rg -n "useTranslation\(\)" src/frontend'

# 13. Page layout contract
#
# Enforced in code-quality check 11 (second scroller, doubled padding) and
# ui-consistency check 12 (self-fixed height). Kept here only as a pointer so
# a reader of this report is not left thinking React standards has no opinion.
Add-SanityCheck -Report $report -Id 13 -Title 'Page layout contract (checked in code-quality 11 / ui-consistency 12)' -Hits @() -Mode info -Severity info `
  -Note 'AppLayout renders PageShell, which owns page padding and is the single scroller. A page adds neither.'

# 14. Retired: the raw div/span/p/h* inventory is reported once, by ui-consistency check 11. The number stays unused so 15+ keep their IDs.

# 15. Flex/Box prop names
$flexBad = @(Find-Pattern @tsx -Multiline -Pattern "<Flex\b$($script:SanityJsxTagBody)\b(items|gapX|flex)=")
Add-SanityCheck -Report $report -Id 15 -Title '<Flex> with items= / gapX= / flex= props' -Hits $flexBad -Mode strict -Severity low -Remediation 'Props are align= and gap=; use className="flex-1" instead of flex=.' -Command 'rg -n "<Flex\s" src/frontend -g "*.tsx"'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
