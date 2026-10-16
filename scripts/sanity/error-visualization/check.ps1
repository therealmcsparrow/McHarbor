# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Error visualization sanity check. Mirrors .instructions/sanity/error-visualization.md.
.DESCRIPTION
  A failure must reach the user as the message the server actually sent, on the
  surface that failed. This check finds the places where it reaches them as
  "Something went wrong" instead, where it is discarded before it can be shown,
  and where it is shown but truncated into a cell.

  Scope is every frontend McHarbor ships: the core app, the plugin frontends,
  and the widget / node catalogs.
.PARAMETER Plugins
  Limit the plugin scope to these slugs (folder names under src/plugins/Plugins).
.EXAMPLE
  .\scripts\sanity\error-visualization\check.ps1
.EXAMPLE
  .\scripts\sanity\error-visualization\check.ps1 -Plugins addon-kvm,addon-compute
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [string[]]$Plugins,
  [int]$MaxHits = 150,
  [string[]]$Paths
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$pluginRoot = 'src/plugins/Plugins'
$pluginPaths = @($pluginRoot)
if ($Plugins -and $Plugins.Count) {
  $pluginPaths = @()
  foreach ($p in $Plugins) { foreach ($part in ($p -split ',')) { $slug = $part.Trim(); if ($slug) { $pluginPaths += "$pluginRoot/$slug" } } }
}

$scopeLabel = 'src/frontend, ' + ($pluginPaths -join ', ') + ', widgets, nodes'
$report = New-SanityReport -Name 'error-visualization' -Title 'Error Visualization Sanity' -Scope $scopeLabel -Stamp $Stamp -Root $Root
Write-SanityHeader $report

# A built UMD bundle is the same source twice; stories and tests are not UI the
# operator ever sees.
$skip = @('\.test\.tsx?$', '\.spec\.tsx?$', '\.stories\.tsx$', '\.umd\.cjs$', '\.d\.ts$', '/dist/', '/\.unused/')
$paths = (Get-SanityScope -Root $Root -Paths $Paths -Default (@('src/frontend') + $pluginPaths + @('widgets', 'nodes')))
$code = @{ Root = $Root; Paths = $paths; Include = @('*.tsx', '*.ts'); ExcludeMatch = $skip }
$tsx = @{ Root = $Root; Paths = $paths; Include = @('*.tsx'); ExcludeMatch = $skip }

# The surfaces that actually put a message in front of the user. A catch, a
# failure callback or a rejected query that reaches none of these is silent.
$surfaces = 'toastError(Details)?\(|toast\.(error|danger|warning)|<ErrorState|<StatusError|<Alert|setError\(|setErrorMessage\(|errorMessage|formError|setFieldError|notifyError|reportError|throw '
# The helpers that carry the server's own words to those surfaces.
$carriers = 'describeApiError|assertSuccess|translateErrorCode|errorDetailsOf|apiErrorOf|toastError|\berr(or)?\s*(instanceof|\.message)|\.error\b|\.code\b'

# ---------------------------------------------------------------------------
# 1. Untranslated literal in an error toast
# ---------------------------------------------------------------------------
$literalToast = @(Find-Pattern @code -Multiline -Pattern 'toast\.(error|danger)\(\s*[''"][^''"]{3,}[''"]')
Add-SanityCheck -Report $report -Id 1 -Title 'Error toast built from a string literal instead of t()' -Hits $literalToast -Mode strict -Severity high `
  -Remediation 'Report a server failure with toastError(err, t(''ns:toast.<what>Failed'')) or toastError(apiErrorOf(res), ...): the operation is the title, the server message and cause sit under it. A literal is English-only and usually generic.' `
  -Command 'rg -n "toast\.error\((''|\")" src/frontend src/plugins/Plugins -g "*.tsx" -g "*.ts" -g "!*.umd.cjs"'

# ---------------------------------------------------------------------------
# 2. Generic message where a specific one exists
# ---------------------------------------------------------------------------
# These keys and literals say "it broke" and nothing else. They are legitimate
# only as the last fallback inside describeApiError; anywhere else they replace
# a message the server already sent.
$genericWords = 'Something went wrong|An (unexpected )?error occurred|Unexpected error|Unknown error|Operation failed|Request failed|An error has occurred'
$genericKeys = 'errors?\.(generic|unknown|unexpected|failed|operationFailed)\b|somethingWentWrong|unexpectedError|unknownError|genericError'
$genericScope = @{ Root = $Root; Paths = $paths; Include = @('*.tsx', '*.ts'); ExcludeMatch = ($skip + @('resources/utils/api-mutation\.ts$', '/i18n/', '/locales/')) }
# `res.error || t('unknownError')` is the generic string doing its one correct
# job - standing behind a specific message that may be absent. The finding is
# the generic string standing *instead of* one.
$genericHits = @(Find-Pattern @genericScope -Pattern "($genericWords|$genericKeys)" `
    -NotMatch '^\s*(//|\*|/\*|\{/\*)|(\.(error|message)|\berr\b)[^;]*(\|\||\?\?)|^\s*return new Error\(.(Request failed|Unknown error).\);?$')
$genericHits = @($genericHits | Where-Object {
    $abs = Join-Path $Root $_.File
    # A route or component error boundary's heading is the last-resort surface; the caught error's own message is rendered beneath it.
    if ((Get-SanityText $abs) -match 'useRouteError|componentDidCatch|getDerivedStateFromError') { return $false }
    # A fallback on its own line behind `x.error ||` on the line above still stands behind a specific message.
    $lines = Get-SanityLines $abs
    if ($_.Line -ge 2 -and $lines[$_.Line - 2] -match '(\.(error|message)|\berr\b)[^;]*(\|\||\?\?)\s*$') { return $false }
    if ($lines[$_.Line - 1] -match '^\s*[a-z0-9]+(_[a-z0-9]+)+\s*:\s*t\(') { return $false }
    $true
  })
Add-SanityCheck -Report $report -Id 2-Title 'Generic "something went wrong" message in a failure path' -Hits $genericHits -Mode review -Severity high `
  -Remediation 'Name the operation and show what the server said: toastError(err, operation) or describeApiError(res), which carry the code, the redacted cause and the request ID. Reserve the generic string for describeApiError''s own last fallback.' `
  -Command 'rg -n "Something went wrong|Unknown error|errors\.generic|unexpectedError" src/frontend src/plugins/Plugins -g "*.tsx" -g "*.ts"'

# ---------------------------------------------------------------------------
# 3. The error object discarded by the failure callback
# ---------------------------------------------------------------------------
# `onError: () => toast.error(t('saveFailed'))` cannot show the reason: the
# argument carrying it was never bound. Taking `(err)` and passing err.message
# costs one word and turns "Save failed" into "Save failed: port 8705 in use".
$blindOnError = @(Find-Pattern @code -Multiline -Pattern '\bon(Error|ErrorToast|Failure)\s*:\s*(async\s*)?\(\s*\)\s*=>(?=[^;]{0,200}?[tT]oast)')
$blindOnError += @(Find-Pattern @code -Multiline -Pattern '\bon(Error|ErrorToast|Failure)\s*\(\s*\)\s*\{')
$blindCatch = @(Find-Pattern @code -Multiline -Pattern '\.catch\(\s*\(\s*\)\s*=>\s*(toast|set[A-Z]|notify)')
Add-SanityCheck -Report $report -Id 3 -Title 'Failure callback takes no error argument' -Hits ($blindOnError + $blindCatch) -Mode review -Severity high `
  -Remediation 'Bind the error and show it: toastError(err, operation), or drop the onError and let the global MutationCache report it. err.message is already the server text plus its cause when the mutation went through assertSuccess().' `
  -Command 'rg -n "onError: \(\) =>" src/frontend src/plugins/Plugins -g "*.tsx" -g "*.ts"'

# ---------------------------------------------------------------------------
# 4. Swallowed failure: an empty catch, or one that only logs
# ---------------------------------------------------------------------------
# Only the catches that do nothing at all, or nothing but log. A catch that
# repairs state - a clipboard fallback, a JSON.parse default, an abort - is
# deliberately silent and is not the subject here.
$catches = @(Find-Pattern @code -Multiline -Pattern 'catch\s*(\([^)]*\))?\s*\{[^{}]*\}')
$swallowed = @($catches | Where-Object {
    $body = $_.Match -replace '^catch\s*(\([^)]*\))?\s*\{', '' -replace '\}$', ''
    foreach ($cm in [regex]::Matches($body, '/\*([\s\S]*?)\*/|(?m)//(.*)$')) {
      if (($cm.Groups[1].Value + $cm.Groups[2].Value).Trim().Length -ge 12) { return $false }
    }
    $body = [regex]::Replace($body, '/\*[\s\S]*?\*/', '')
    $body = [regex]::Replace($body, '(?m)//.*$', '')
    $body = [regex]::Replace($body, 'console\.\w+\([^()]*\)\s*;?', '')
    [string]::IsNullOrWhiteSpace($body)
  })
Add-SanityCheck -Report $report -Id 4 -Title 'catch block that reports nothing, or logs and nothing else' -Hits $swallowed -Mode review -Severity medium `
  -Remediation 'Either surface it (toast.error / <ErrorState> / setError) or recover with an explicit fallback value. A console.error alone is invisible to the operator, and an empty catch is invisible to everyone.' `
  -Command 'rg -n -U "catch\s*(\([^)]*\))?\s*\{[^{}]*\}" src/frontend -g "*.tsx"'

# ---------------------------------------------------------------------------
# 5. A rejected query that never reaches the screen
# ---------------------------------------------------------------------------
# TanStack hands back `isError` and `error`; a page that reads neither renders
# an empty table for a 500 and an empty table for "no rows".
$queryPages = @(Find-FilesWithoutMatch -Root $Root -Paths $paths -Include @('*Page.tsx', '*Panel.tsx', '*Tab.tsx') -ExcludeMatch $skip `
    -Pattern 'isError|isLoadingError|<ErrorState|<StatusError|\berror\s*(&&|\?)|error:|\berror=\{' `
    -OnlyIfMatch 'useQuery\(|useInfiniteQuery\(|useSuspenseQuery\(|use[A-Z]\w*Query\(')
Add-SanityCheck -Report $report -Id 5 -Title 'Page or panel with queries but no failure state' -Hits $queryPages -Mode review -Severity high `
  -Remediation 'Render <ErrorState variant="card" error={error} onRetry={refetch} /> when isError. An empty table must mean "nothing here", never "the request failed".' `
  -Command 'rg -L "isError|<ErrorState" src/frontend/modules -g "*Page.tsx"'

# ---------------------------------------------------------------------------
# 6. A failed row without <StatusError>
# ---------------------------------------------------------------------------
# .claude/rules/table-status-errors.md: the row says that it failed, the dialog
# says why.
$rowErrorFiles = @()
foreach ($f in @(Get-SanityFiles @tsx)) {
  $text = Get-SanityText $f.Full
  if ($text -notmatch '\b(lastError|last_error|errorMessage|error_message|failureReason)\b') { continue }
  if ($text -notmatch 'accessorKey|columns\s*[:=]|<DataGridTable|<TablePage|<DataTable|ColumnDef') { continue }
  if ($text -match '<StatusError') { continue }
  # The cells may live in a sibling columns module this file imports; that module is where <StatusError> belongs.
  $columnsImports = @([regex]::Matches($text, "from\s+['""](\./[^'""]*Columns[^'""]*)['""]") | ForEach-Object { $_.Groups[1].Value })
  $columnsCarryIt = $false
  foreach ($imp in $columnsImports) {
    $base = Join-Path (Split-Path -Parent $f.Full) $imp
    foreach ($ext in @('.tsx', '.ts')) {
      if ((Test-Path -LiteralPath ($base + $ext)) -and ((Get-SanityText ($base + $ext)) -match '<StatusError')) { $columnsCarryIt = $true }
    }
  }
  if ($columnsCarryIt) { continue }
  $rowErrorFiles += New-SanityHit -File $f.Rel -Line 0 -Text 'table carries a row error field but renders no <StatusError>'
}
Add-SanityCheck -Report $report -Id 6 -Title 'Table with a row error field and no <StatusError>' -Hits $rowErrorFiles -Mode review -Severity high `
  -Remediation 'Put <StatusError error={row.original.lastError} /> next to the state badge. It renders nothing when the field is blank, so pass it unconditionally.' `
  -Command 'rg -l "lastError|last_error" src/frontend src/plugins/Plugins -g "*.tsx" | xargs rg -L "StatusError"'

# ---------------------------------------------------------------------------
# 7. The message shown, but cut short
# ---------------------------------------------------------------------------
# A truncated error is worse than none: the first forty characters name the
# command, the rest named the cause.
$sliced = @(Find-Pattern @code -Pattern '\b(error|lastError|errorMessage|err)\w*\s*\.\s*(slice|substring|substr)\s*\(' -IgnoreCase)
# title={t('...lastError')} is a section heading; title={row.lastError} is the
# dialog replaced by a tooltip nobody on a touch screen can open.
$tooltipOnly = @(Find-Pattern @tsx -Pattern 'title=\{[^}]*\b(error|lastError|errorMessage)\b' -IgnoreCase -NotMatch '\bt\(|i18n')
$clamped = @(Find-Pattern @tsx -Pattern '\b(truncate|line-clamp-\d|text-ellipsis)\b' -AlsoMatch '\b(error|lastError|errorMessage)\b')
Add-SanityCheck -Report $report -Id 7 -Title 'Error text truncated, clamped, or hidden in a title tooltip' -Hits ($sliced + $tooltipOnly + $clamped) -Mode strict -Severity high `
  -Remediation 'Show the whole message behind <StatusError> (or <ErrorState>), which wraps and scrolls it and offers a copy button. A title tooltip is not a substitute for the dialog.' `
  -Command 'rg -n "truncate|line-clamp|\.slice\(0," src/frontend -g "*.tsx" | rg "error"'

# ---------------------------------------------------------------------------
# 8. Browser dialogs standing in for an error surface
# ---------------------------------------------------------------------------
$nativeDialogs = @(Find-Pattern @code -Pattern '(^|[^.\w])(window\.)?(alert|confirm|prompt)\s*\(' -NotMatch 'Dialog|//|confirmPassword|\bprompt(s)?\b\s*[:=]')
Add-SanityCheck -Report $report -Id 8 -Title 'window.alert / confirm used instead of the app''s own surfaces' -Hits $nativeDialogs -Mode strict -Severity medium `
  -Remediation 'Use toast.error / <ErrorState> for failures and <ConfirmDialog> for confirmations. A native dialog is untranslated, unstyled, and blocks the tab.' `
  -Command 'rg -n "window\.alert|[^.]alert\(|[^.]confirm\(" src/frontend src/plugins/Plugins -g "*.tsx" -g "*.ts"'

# ---------------------------------------------------------------------------
# 9. Errors that only reach the console
# ---------------------------------------------------------------------------
$consoleOnly = @()
foreach ($f in @(Get-SanityFiles @code)) {
  $text = Get-SanityText $f.Full
  if ($text -notmatch 'console\.(error|warn)\(') { continue }
  if ($text -match $surfaces) { continue }
  # A boundary's fallback UI is the surface, and a toast bridge is the surface
  # other files reach for; neither is a file that only logs.
  if ($text -match 'componentDidCatch|getDerivedStateFromError') { continue }
  # A React 19 root guard renders its own fallback from onUncaughtError, and a hook
  # that hands a failure count to its caller leaves the surface to that caller.
  if ($text -match 'onUncaughtError[\s\S]*\.render\(|\bfailedSources\b') { continue }
  if ($f.Name -match '(?i)toast') { continue }
  $n = ([regex]::Matches($text, 'console\.(error|warn)\(')).Count
  # A console call behind an import.meta.env.DEV guard is a developer diagnostic that ships in no production bundle.
  if (([regex]::Matches($text, 'import\.meta\.env\.DEV')).Count -ge $n) { continue }
  $consoleOnly += New-SanityHit -File $f.Rel -Line 0 -Text "$n console.error/warn call(s), no user-visible error surface in the file"
}
Add-SanityCheck -Report $report -Id 9 -Title 'File logs failures to the console and nowhere else' -Hits $consoleOnly -Mode review -Severity medium `
  -Remediation 'The console is for the developer; the operator needs the toast or the panel. Keep the log and add the surface, or let the error propagate to a caller that has one.' `
  -Command 'rg -l "console\.error" src/frontend -g "*.tsx" | xargs rg -L "toast\.error|ErrorState|setError"'

# ---------------------------------------------------------------------------
# 10. The API envelope thrown away
# ---------------------------------------------------------------------------
# `{ success, error?, code? }` is the whole point of the response contract: the
# handler resolved an i18n code for this exact failure. A branch on !success
# that ignores both fields discards it.
$envelope = @(Find-Pattern @code -Multiline -Pattern 'if\s*\(\s*!\s*\w+\.success\s*\)\s*\{[^{}]{0,240}\}' `
    -NotMatch 'describeApiError|assertSuccess|translateErrorCode|apiErrorOf|\.error|\.code|\.message')
Add-SanityCheck -Report $report -Id 10 -Title '!res.success handled without reading error / code' -Hits $envelope -Mode review -Severity high `
  -Remediation 'Use assertSuccess(res) (throws the resolved text) or describeApiError(res) from @resources/utils/api-mutation. Both turn the handler''s i18n code into the operator''s language.' `
  -Command 'rg -n -U "if \(!\w+\.success\)" src/frontend src/plugins/Plugins -g "*.tsx" -g "*.ts"'

# ---------------------------------------------------------------------------
# 11. Mutations with no failure path at all
# ---------------------------------------------------------------------------
$noFailurePath = @(Find-FilesWithoutMatch -Root $Root -Paths $paths -Include @('*.tsx', '*.ts') -ExcludeMatch $skip `
    -Pattern 'onError|meta\s*:|catch\s*[({]|isError|onErrorToast|\.error\s*(&&|\?)' -OnlyIfMatch 'useMutation\(')
# A plugin bundle that installs its own MutationCache toasts every rejected mutation (server text), so its hooks need no onError.
$noFailurePath = @($noFailurePath | Where-Object {
    if ($_.File -notmatch '^(src/plugins/Plugins/[^/]+)/frontend/') {
      # main.tsx's MutationCache toasts every rejected host mutation with error.message, so a mutationFn that ends in
      # assertSuccess / refusalOf already has its failure path; only a mutation that never throws the envelope is silent.
      $hostText = [regex]::Replace((Get-SanityText (Join-Path $Root $_.File)), '(?ms)^import\b.*?;\s*$', '')
      $mutations = ([regex]::Matches($hostText, 'useMutation\(')).Count
      $throwing = ([regex]::Matches($hostText, 'assertSuccess|refusalOf|throw new Error')).Count
      return ($throwing -lt $mutations)
    }
    $entry = Join-Path $Root ($Matches[1] + '/frontend/src/index.tsx')
    -not ((Test-Path -LiteralPath $entry) -and ((Get-Content -LiteralPath $entry -Raw) -match '(?i)mutationCache'))
  })
Add-SanityCheck -Report $report -Id 11 -Title 'useMutation with no onError, meta.error or catch' -Hits $noFailurePath -Mode review -Severity high `
  -Remediation 'A mutation that can fail needs a failure path: onError, meta: { error }, or an await in a try/catch that surfaces the message. Silence looks identical to success.' `
  -Command 'rg -l "useMutation\(" src/frontend src/plugins/Plugins -g "*.tsx" | xargs rg -L "onError|meta:"'

# ---------------------------------------------------------------------------
# 12. Plugin frontends that never carry the server's words
# ---------------------------------------------------------------------------
# A plugin bundle compiles the host's helpers in like any other import. One that
# reports failures without ever touching describeApiError / assertSuccess /
# ErrorState / StatusError is inventing its own generic text.
$pluginHits = @()
$pluginDirs = @()
foreach ($p in $pluginPaths) {
  $full = Join-Path $Root $p
  if (-not (Test-Path -LiteralPath $full)) { continue }
  if ((Split-Path -Leaf $p) -ne 'Plugins') { $pluginDirs += (Get-Item -LiteralPath $full); continue }
  $pluginDirs += @(Get-ChildItem -LiteralPath $full -Directory)
}
foreach ($d in $pluginDirs) {
  $src = Join-Path $d.FullName 'frontend\src'
  if (-not (Test-Path -LiteralPath $src)) { continue }
  $files = @(Get-SanityFiles -Root $Root -Paths @($src) -Include @('*.tsx', '*.ts') -ExcludeMatch $skip)
  if ($files.Count -eq 0) { continue }
  $reports = $false
  $carries = $false
  foreach ($f in $files) {
    $text = Get-SanityText $f.Full
    if ($text -match 'toast\.(error|danger)|isError|setError\(') { $reports = $true }
    if ($text -match 'describeApiError|assertSuccess|<ErrorState|<StatusError') { $carries = $true }
  }
  if ($reports -and -not $carries) {
    $pluginHits += New-SanityHit -File (Get-SanityRelPath -Root $Root -Full $src) -Line 0 -Text 'reports failures but never uses describeApiError / assertSuccess / ErrorState / StatusError'
  }
}
Add-SanityCheck -Report $report -Id 12 -Title 'Plugin frontend reporting failures without the host error helpers' -Hits $pluginHits -Mode review -Severity medium `
  -Remediation 'Import describeApiError / assertSuccess from @resources/utils/api-mutation and render <ErrorState> / <StatusError>. The helpers resolve through the host i18n instance, which a plugin''s own bundled i18next cannot.' `
  -Command 'rg -l "toast\.error" src/plugins/Plugins/*/frontend/src | xargs rg -L "describeApiError|assertSuccess"'

# ---------------------------------------------------------------------------
# 13. Boundary coverage: 13a host (must reach 0), 13b plugins (inventory)
# ---------------------------------------------------------------------------
# A mount is guarded when the file renders an ErrorBoundary / PluginMountBoundary
# element (file-level proxy for "inside or under"), when an imperative .mount() sits
# in a try block, or when the createRoot call passes onUncaughtError.
$boundaryRx = New-Object System.Text.RegularExpressions.Regex('<\s*(\w*ErrorBoundary|PluginMountBoundary)\b')
$mountRx = New-Object System.Text.RegularExpressions.Regex('\.mount\(|createRoot\(|React\.lazy\(|\blazy\(')
$hostMounts = New-Object System.Collections.Generic.List[object]
$pluginMounts = New-Object System.Collections.Generic.List[object]
$guardedMounts = 0
$boundaryRefs = 0
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $paths -Include @('*.tsx') -ExcludeMatch $skip)) {
  $text = Get-SanityText $f.Full
  if ([string]::IsNullOrEmpty($text)) { continue }
  $boundaryRefs += ([regex]::Matches($text, 'ErrorBoundary|PluginMountBoundary')).Count
  $hasBoundary = $boundaryRx.IsMatch($text)
  foreach ($m in $mountRx.Matches($text)) {
    $lineStart = $text.LastIndexOf("`n", [Math]::Max(0, $m.Index - 1)) + 1
    $lineEnd = $text.IndexOf("`n", $m.Index); if ($lineEnd -lt 0) { $lineEnd = $text.Length }
    if ($text.Substring($lineStart, $lineEnd - $lineStart) -match '^\s*(//|\*|/\*|\{/\*)') { continue }
    $guarded = $hasBoundary
    # A lazy() of first-party code is a code-split chunk, not a foreign bundle; the route's
    # errorElement already catches it. Only a lazy() of something else counts here.
    if (-not $guarded -and $m.Value -like '*lazy(') {
      $win = $text.Substring($m.Index, [Math]::Min(200, $text.Length - $m.Index))
      $guarded = ($win -match "import\(\s*['""](\.{1,2}/|@resources/|@core/|@modules/|@widgets/|@nodes/)")
    }
    # The application root sits above the router whose errorElement guards every page;
    # there is nothing above it to recover into.
    if (-not $guarded -and $m.Value -like '*createRoot*' -and $f.Rel -eq 'src/frontend/main.tsx') { $guarded = $true }
    if (-not $guarded -and $m.Value -like '*createRoot*') {
      $win = $text.Substring($m.Index, [Math]::Min(700, $text.Length - $m.Index))
      $guarded = ($win -match 'onUncaughtError')
    }
    if (-not $guarded -and $m.Value -eq '.mount(') {
      # Walk back to each enclosing '{' by brace depth and ask whether it opens a try block.
      $depth = 0
      $stop = [Math]::Max(0, $m.Index - 6000)
      for ($k = $m.Index - 1; $k -ge $stop -and -not $guarded; $k--) {
        $ch = $text[$k]
        if ($ch -eq '}') { $depth++ }
        elseif ($ch -eq '{') {
          if ($depth -gt 0) { $depth-- }
          else { $guarded = ($text.Substring([Math]::Max(0, $k - 12), [Math]::Min(12, $k)) -match '\btry\s*$') }
        }
      }
    }
    if ($guarded) { $guardedMounts++; continue }
    $hit = New-SanityHit -File $f.Rel -Line (Get-SanityLineNumber -Text $text -Index $m.Index) -Text $m.Value
    if ($f.Rel -like 'src/plugins/*') { $pluginMounts.Add($hit) } else { $hostMounts.Add($hit) }
  }
}
$hostFileRows = @($hostMounts | Group-Object File | Sort-Object Count -Descending | ForEach-Object { New-SanityHit -File $_.Name -Line 0 -Text ("{0} unguarded mount point(s)" -f $_.Count) })
$pluginFileRows = @($pluginMounts | Group-Object File | Sort-Object Count -Descending | ForEach-Object { New-SanityHit -File $_.Name -Line 0 -Text ("{0} unguarded mount point(s)" -f $_.Count) })
Add-SanityCheck -Report $report -Id '13a' -Title 'Host mount points with no error boundary' -Hits $hostFileRows -Mode review -Severity medium `
  -Remediation 'Wrap the plugin/widget mount container in <PluginMountBoundary> (@resources/components) so a bundle that throws while rendering shows <ErrorState> instead of taking the page down; wrap an imperative mount() in try/catch that renders the same state. This section must reach 0.' `
  -Note ("{0} unguarded host mount call(s) in {1} file(s); {2} guarded mount(s) and {3} ErrorBoundary/PluginMountBoundary reference(s) across the scanned tree. Guarded = the file renders a boundary element, an imperative .mount() sits in a try block, or createRoot passes onUncaughtError." -f $hostMounts.Count, $hostFileRows.Count, $guardedMounts, $boundaryRefs) `
  -Command 'rg -n "\.mount\(|createRoot\(|lazy\(" src/frontend widgets nodes -g "*.tsx"'
Add-SanityCheck -Report $report -Id '13b' -Title 'Plugin frontend roots with no error boundary (inventory)' -Hits $pluginFileRows -Mode info -Severity info `
  -Remediation 'A plugin''s own createRoot cannot be caught by a host React boundary and plugins share no SDK, so each plugin passes onUncaughtError to createRoot (or renders its own boundary). Batched with the next plugin rebuild; new plugins start guarded from the mcharbor plugin dev new scaffold.' `
  -Note ("{0} unguarded plugin mount call(s) in {1} file(s)." -f $pluginMounts.Count, $pluginFileRows.Count) `
  -Command 'rg -n "createRoot\(" src/plugins/Plugins/*/frontend/src'

Add-SanityNote -Report $report -Text 'Accepted: describeApiError() in resources/utils/api-mutation.ts owns the one legitimate generic fallback; the locale bundles legitimately define the generic strings it falls back to.'
Add-SanityNote -Report $report -Text 'Accepted: a catch that returns an explicit fallback value (clipboard, JSON.parse, AbortError) is recovering, not swallowing, and is not reported by check 4.'
Add-SanityNote -Report $report -Text 'Built plugin bundles (*.umd.cjs), stories, tests and dist/ output are excluded - they are the same source compiled.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
