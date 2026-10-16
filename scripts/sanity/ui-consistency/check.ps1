# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  UI consistency sanity check. Mirrors .instructions/sanity/ui-consistency.md.
.EXAMPLE
  .\scripts\sanity\ui-consistency\check.ps1
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

$report = New-SanityReport -Name 'ui-consistency' -Title 'UI Consistency Sanity' -Scope 'src/frontend, src/plugins/Plugins/*/frontend/src' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$feDefault = @('src/frontend') + @(Get-SanityPluginPaths -Root $Root -SubPath 'frontend/src')
$feScope = (Get-SanityScope -Root $Root -Paths $Paths -Default $feDefault)
$pagesScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend/modules'))
$panelScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend/modules', 'src/frontend/resources'))
$tsx = @{ Root = $Root; Paths = $feScope; Include = @('*.tsx'); ExcludeMatch = @('\.test\.tsx$', '\.stories\.tsx$') }
$pages = @{ Root = $Root; Paths = $pagesScope; Include = @('*Page.tsx') }

# 1. Shared button component usage
$rawButtons = @(Find-Pattern -Root $Root -Paths $feScope -Include @('*.tsx') -Pattern '<button\b' -NotMatch '^\s*(\*|//|/\*)' -ExcludeMatch @('\.test\.tsx$', '\.stories\.tsx$', 'resources/components/ui/'))
# A plugin whose vite config has no @resources alias cannot import the host Button; its native buttons are the only option.
$rawButtons = @($rawButtons | Where-Object {
    if ($_.File -notmatch '^(src/plugins/Plugins/[^/]+)/frontend/') { return $true }
    $cfg = Join-Path $Root ($Matches[1] + '/frontend/vite-plugin.config.ts')
    -not ((Test-Path -LiteralPath $cfg) -and ((Get-Content -LiteralPath $cfg -Raw) -notmatch "'@resources'\s*:"))
  })
Add-SanityCheck -Report $report -Id 1 -Title 'Raw <button> outside resources/components/ui' -Hits $rawButtons -Mode strict -Severity medium `
  -Remediation 'Use <Button> from @resources/components/ui/Button. Low-level primitives inside components/ui are exempt.' -Command 'rg -n "<button" src/frontend -g "*.tsx"'

# 2. Page chrome reuse
#
# <PageHeader> is one of three shared forms, not the only one: a table page
# passes `title` to <TablePage>, and a detail page puts its orientation in the
# topbar via <HeaderBreadcrumb> so it keeps the same coordinates on every
# screen. All three are the shared chrome; a hand-built <h1> is not.
$noPageHeader = @(Find-FilesWithoutMatch @pages -Pattern '<PageHeader|<TablePage|<HeaderBreadcrumb|<PageToolbar|useHeaderSlot|min-h-screen')
# A *Page.tsx that another page lazy-loads is a tab body, not a route: its
# chrome belongs to the page hosting the tabs.
$pageSources = @{}
foreach ($p in @(Get-SanityFiles @pages)) { $pageSources[$p.Rel] = Get-SanityText $p.Full }
$noPageHeader = @($noPageHeader | Where-Object {
    # A module that ships its own shell (modules/<m>/layout/*Layout.tsx, e.g. the phone-first on-call shell) is not hosted
    # by AppLayout/PageShell, so the desktop page chrome does not apply to its pages.
    if (Get-ChildItem -Path (Join-Path $Root (($_.File -replace '/pages/.*$', '') + '/layout')) -Filter '*Layout.tsx' -ErrorAction SilentlyContinue) { return $false }
    $name =($_.File -replace '^.*/', '') -replace '\.tsx$', ''
    $hosted = $false
    foreach ($kv in $pageSources.GetEnumerator()) {
      if ($kv.Key -eq $_.File) { continue }
      if ($kv.Value -match ('import\(["''][^"'']*/' + [regex]::Escape($name) + '["'']\)')) { $hosted = $true; break }
    }
    -not $hosted
  })
Add-SanityCheck -Report $report -Id 2 -Title 'Pages with no shared page chrome' -Hits $noPageHeader -Mode review -Severity low `
  -Remediation 'Use <PageHeader>, <TablePage title=...> or <HeaderBreadcrumb> instead of a hand-built heading.' -Command 'rg -n "<PageHeader|<TablePage|<HeaderBreadcrumb" src/frontend/modules -g "*Page.tsx"'

# 3. Design token drift
$tokens = @(Find-Pattern -Root $Root -Paths $feScope -Include @('*.tsx', '*.ts') -ExcludeMatch @('\.test\.', '\.stories\.', 'canvas-theme\.ts$') -Pattern '\b(bg-white|bg-black|text-white|text-black|text-gray-\d+|bg-gray-\d+|border-gray-\d+)\b')
Add-SanityCheck -Report $report -Id 3 -Title 'Hardcoded utility colours bypassing theme tokens' -Hits $tokens -Mode strict -Severity medium `
  -Remediation 'Use semantic OKLCH tokens: bg-card, bg-background, text-foreground, text-muted-foreground, border-border, text-primary-foreground.' `
  -Command 'rg -n "bg-white|bg-black|text-white|text-black|text-gray-|bg-gray-|border-gray-" src/frontend -g "*.tsx" -g "*.ts"'

# 4. Icon library drift
$icons = @(Find-Pattern -Root $Root -Paths $feScope -Include @('*.tsx', '*.ts') -Pattern "from ['`"](@heroicons|lucide-react|react-icons|@fortawesome|react-feather)")
Add-SanityCheck -Report $report -Id 4 -Title 'Icons imported from a library other than @tabler/icons-react' -Hits $icons -Mode strict -Severity medium -Remediation 'Import from @tabler/icons-react.' -Command "rg -n `"from '@heroicons|from 'lucide-react'|from 'react-icons'`" src/frontend"

# 5. Inline styles
$inline = @(Find-Pattern @tsx -Pattern 'style=\{\{')
Add-SanityCheck -Report $report -Id 5 -Title 'Inline style objects' -Hits $inline -Mode strict -Severity medium -Remediation 'Use Tailwind utilities or arbitrary values (w-[240px]); dynamic values go through CSS variables.' -Command 'rg -n "style=\{\{" src/frontend -g "*.tsx"'

# 6. Spacing pattern drift
# The rule is about *cards and panels*, so the padding only counts when the
# same className says it is one. `mc-glass` and `bg-card` are the card
# vocabulary; a `bg-muted/30` or `bg-background` box inside a dialog is an
# inset sub-panel, and p-3 on a compact row, a chip or a toolbar is the scale
# being used rather than ignored. `rounded-lg` / `rounded-xl` is the card
# radius; `rounded-md` on the same surface is a row or a chip, which is
# allowed to be denser.
$spacing = @(Find-Pattern -Root $Root -Paths $panelScope -Include @('*.tsx') `
    -Pattern 'className="[^"]*\b(p-3|p-5|p-7)\b' `
    -AlsoMatch 'rounded-(lg|xl)[^"]*(\bmc-glass\b|\bbg-card\b)|(\bmc-glass\b|\bbg-card\b)[^"]*rounded-(lg|xl)')
Add-SanityCheck -Report $report -Id 6 -Title 'Odd padding on a card or panel (p-3 / p-5 / p-7)' -Hits $spacing -Mode review -Severity low -Remediation 'Cards and panels use p-4 or p-6.' -Command 'rg -n "className=\"[^\"]*(p-3|p-5|p-7)" src/frontend/modules src/frontend/resources -g "*.tsx"'

# 7. Destructive button variants
$destructiveTags = @(Find-Pattern @tsx -Multiline -Pattern "<Button\b$($script:SanityJsxTagBody)>\s*(\{[^}]*\}\s*)?[^<{]*\b(Delete|Remove|Destroy|Purge|Wipe)\b")
$destructive = @($destructiveTags | Where-Object { $_.Match -notmatch 'variant=["'']?(destructive|danger)' -and $_.Match -notmatch '\bt\(' })
Add-SanityCheck -Report $report -Id 7 -Title 'Delete/Remove buttons without variant="destructive"' -Hits $destructive -Mode review -Severity medium `
  -Remediation 'Destructive actions use variant="destructive" (and a confirmation dialog).' -Command 'rg -n "<Button[^>]*>([^<]*(Delete|Remove|Destroy))" src/frontend -g "*.tsx"'

# 8. Icon-only button accessibility
$iconTags = @(Find-Pattern @tsx -Multiline -Pattern "<(Button|IconButton|HeaderActionButton|ActionButton)\b$($script:SanityJsxTagBody)\bsize=[`"']icon(-sm|-xs|-lg)?[`"']$($script:SanityJsxTagBody)>")
$iconNoLabel = @($iconTags | Where-Object { $_.Match -notmatch 'aria-label' -and $_.Match -notmatch '\{\.\.\.' -and $_.Match -notmatch '\btitle=' })
Add-SanityCheck -Report $report -Id 8 -Title 'Icon-only buttons without aria-label' -Hits $iconNoLabel -Mode strict -Severity medium `
  -Remediation 'Add aria-label={t(...)} to every size="icon" button.' -Command 'rg -n "<Button[^>]*size=\"icon\"" src/frontend -g "*.tsx"'

# 9. Loading states
$noLoading = @()
foreach ($f in @(Get-SanityFiles @pages)) {
  $text = Get-SanityText $f.Full
  if ($text -notmatch 'useQuery|use\w+Query\(|use\w+List\(|use\w+Detail\(') { continue }
  if ($text -notmatch 'isLoading|isPending|isFetching|Skeleton|Spinner|LoadingState|loading') { $noLoading += New-SanityHit -File $f.Rel -Line 0 -Text 'data hooks used but no loading state surfaced' }
}
Add-SanityCheck -Report $report -Id 9 -Title 'Data-heavy pages without a loading state' -Hits $noLoading -Mode review -Severity medium -Remediation 'Render a skeleton or spinner while isLoading/isPending.' -Command 'rg -n "isLoading|isPending" src/frontend/modules -g "*Page.tsx"'

# 10. Responsive design
# A page that renders its columns through <TablePage> / <DataTable> inherits
# their breakpoints; the finding is a page that lays out its own grid.
# flex-wrap and an overflow-x wrapper are responsive strategies of their own: the row reflows, or the table scrolls.
$noResponsive = @(Find-FilesWithoutMatch @pages -Pattern '\b(sm|md|lg|xl):|<TablePage|<DataTable|<TableFilterBar|overflow-x-auto|flex-wrap' -OnlyIfMatch 'grid-cols|columns|<Table|DataTable|flex-row')
$noResponsive = @($noResponsive | Where-Object {
    # Pages of a module with its own shell are laid out for that shell's width (the on-call shell is phone-only).
    -not (Get-ChildItem -Path (Join-Path $Root (($_.File -replace '/pages/.*$', '') + '/layout')) -Filter '*Layout.tsx' -ErrorAction SilentlyContinue)
  })
Add-SanityCheck -Report $report -Id 10 -Title 'Multi-column pages with no responsive breakpoints' -Hits $noResponsive -Mode review -Severity low `
  -Remediation 'Add sm:/md:/lg: variants to grids and control rows. Pages that lay out through TablePage/DataTable inherit theirs.' -Command 'rg -L "sm:|md:|lg:" src/frontend/modules -g "*Page.tsx"'

# 11. Centralized layout components
$rawDiv = @(Find-Pattern @tsx -Pattern '<div\s' -NotMatch 'Box|Flex|className=.flex')
$rawSpan = @(Find-Pattern @tsx -Pattern '<span\s' -NotMatch 'Box|Text|Caption|Kbd')
$divTop = @($rawDiv | Group-Object File | Sort-Object Count -Descending | Select-Object -First 25 | ForEach-Object { "$($_.Name): $($_.Count) raw <div>" })
$spanTop = @($rawSpan | Group-Object File | Sort-Object Count -Descending | Select-Object -First 25 | ForEach-Object { "$($_.Name): $($_.Count) raw <span>" })
Add-SanityCheck -Report $report -Id 11 -Title 'Raw <div>/<span> instead of Box/Flex/Text (top files)' -Hits ($divTop + $spanTop) -Mode info -Severity low `
  -Note ("Totals: {0} raw <div>, {1} raw <span> across src/frontend. Listed are the 25 densest files per element." -f $rawDiv.Count, $rawSpan.Count) -Command 'rg -n "<div\s" src/frontend -g "*.tsx" | Select-String -NotMatch "Box|Flex|className=.flex"'

# 12. Page layout contract
#
# PageShell (rendered by AppLayout) owns the page's padding and is the one
# scroller on the screen; PageLayout, which used to be asked for here, was a
# leftover that nested a second scroller inside it. See code-quality check 11
# for the enforced form. What is left for this check is the page's own frame:
# `h-full` on a page root fixes its height inside a shell that already sizes
# it, which is how a page ends up with content it cannot reach.
$fixedHeightRoot = @(Find-Pattern @pages -Multiline -Pattern 'return \(\s*<(div|Box)[^>]*className="[^"]*h-full[^"]*"' -NotMatch 'min-h-0')
Add-SanityCheck -Report $report -Id 12 -Title 'Page root fixing its own height without min-h-0' -Hits $fixedHeightRoot -Mode review -Severity medium `
  -Remediation 'A page root inside PageShell sizes itself with `flex min-h-0 flex-col`; `h-full` alone clips overflow instead of letting the shell scroll.' `
  -Command 'rg -n "h-full" src/frontend/modules -g "*Page.tsx"'

# 13. Flex component prop API
$flexItems = @(Find-Pattern @tsx -Multiline -Pattern "<Flex\b$($script:SanityJsxTagBody)\bitems=")
$flexProp = @(Find-Pattern @tsx -Multiline -Pattern "<Flex\b$($script:SanityJsxTagBody)\bflex=")
$flexGapX = @(Find-Pattern @tsx -Multiline -Pattern "<Flex\b$($script:SanityJsxTagBody)\bgapX=")
Add-SanityCheck -Report $report -Id 13 -Title '<Flex> with items= / flex= / gapX= props' -Hits ($flexItems + $flexProp + $flexGapX) -Mode strict -Severity low `
  -Remediation 'The props are align= and gap=; use className="flex-1" instead of flex=.' -Command 'rg -n "<Flex\s[^>]*items=" src/frontend -g "*.tsx"'

Add-SanityNote -Report $report -Text 'Accepted: the Button primitive itself and other low-level controls under resources/components/ui; canvas-theme.ts owns workflow canvas colours.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
