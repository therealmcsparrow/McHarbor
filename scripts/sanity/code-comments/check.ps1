# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Code-comment sanity check. Mirrors .instructions/sanity/code-comments.md.
.DESCRIPTION
  Reports every comment in non-Go source that the rule does not allow
  (.claude/rules/code-comments.md). Allowed without asking: the copyright header,
  toolchain directives, and Stub/TODO/FIXME markers.

  Go is out of scope for the report by design: .go files are documented to the Go
  doc-comment standard, so no Go comment is ever a finding here. Go comments are
  reviewed under go-standards instead. .go files are still swept for
  Stub/TODO/FIXME markers, because a marker is unfinished work rather than a
  question of comment style.

  Toolchain directives are skipped silently. Markers from every language are
  written to .references/todo/codefixes/<stamp>-stub-todo-fixme.md instead of the
  report.

  Files are tokenised, not grepped, so a '//' inside a string, template literal
  or URL is not reported.
.PARAMETER Changed
  Scope to files changed in the working tree (git status --porcelain).
.PARAMETER Since
  Scope to files changed against a git ref (git diff --name-only <ref>...HEAD).
.EXAMPLE
  .\scripts\sanity\code-comments\check.ps1
.EXAMPLE
  .\scripts\sanity\code-comments\check.ps1 -Changed
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [string[]]$Paths,
  [switch]$Changed,
  [string]$Since,
  [string]$TodoDir = '.references/todo/codefixes'
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$defaultPaths = @('src/backend', 'src/frontend', 'widgets', 'nodes')
$include = @('*.css', '*.sql')
$goDefaultPaths = @('src/backend', 'src/frontend', 'src/agent', 'src/cli', 'src/terraform-provider-mcharbor', 'src/plugins', 'widgets', 'nodes')
$goInclude = @('*.go', '*.ts', '*.tsx')
$excludeMatch = @('/vendor/', '/node_modules/', '/dist/', '/coverage/', '\.min\.(js|css)$', '\.d\.ts$')

$copyrightRx = New-Object System.Text.RegularExpressions.Regex('Copyright \(c\)|licensed under the McHarbor License')
$directiveRx = New-Object System.Text.RegularExpressions.Regex('^\s*(@ts-expect-error|@ts-nocheck|@ts-ignore|eslint|oxlint|prettier-ignore|biome-ignore|@vite-ignore|@jsxImportSource|/ <reference|!)')
$markerRx = New-Object System.Text.RegularExpressions.Regex('^\s*(Stub|TODO|FIXME|XXX|HACK)\s*(\(|:|-|$)', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
$markerPrefilterRx = New-Object System.Text.RegularExpressions.Regex('\b(Stub|TODO|FIXME|XXX|HACK)\b', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)

function Get-GitScopedFiles {
  param([string]$Root, [switch]$Changed, [string]$Since)
  $git = Resolve-SanityTool -Name 'git'
  if (-not $git) { return $null }
  if ($Since) {
    $r = Invoke-SanityCommand -FilePath $git -Arguments ("diff --name-only {0}...HEAD" -f $Since) -WorkingDirectory $Root -TimeoutSec 60
    if ($r.Failed -or $r.ExitCode -ne 0) { return $null }
    return @($r.StdOut -split "`r?`n" | Where-Object { $_ })
  }
  $r = Invoke-SanityCommand -FilePath $git -Arguments 'status --porcelain --untracked-files=all' -WorkingDirectory $Root -TimeoutSec 120
  if ($r.Failed -or $r.ExitCode -ne 0) { return $null }
  $out = New-Object System.Collections.Generic.List[string]
  foreach ($line in ($r.StdOut -split "`r?`n")) {
    if ($line.Length -lt 4) { continue }
    $p = $line.Substring(3).Trim('"')
    if ($p -match ' -> ') { $p = ($p -split ' -> ')[1] }
    $out.Add($p)
  }
  return $out.ToArray()
}

$scopeNote = ''
if ($Paths -and $Paths.Count) {
  $searchPaths = $Paths
  $scopeNote = 'explicit -Paths'
} elseif ($Changed -or $Since) {
  $gitFiles = Get-GitScopedFiles -Root $Root -Changed:$Changed -Since $Since
  if ($null -eq $gitFiles) {
    $searchPaths = $defaultPaths
    $scopeNote = 'git unavailable, fell back to the full tree'
  } else {
    $searchPaths = @($gitFiles | Where-Object { $_ -match '\.(ts|tsx|css|sql|go)$' } | Where-Object { Test-Path -LiteralPath (Join-Path $Root $_) })
    $scopeNote = if ($Since) { "changed against $Since" } else { 'working-tree changes' }
  }
} else {
  $searchPaths = $defaultPaths
  $scopeNote = 'full tree'
}

$report = New-SanityReport -Name 'code-comments' -Title 'Code Comment Sanity' `
  -Scope ("{0} ({1}) - .css .sql, excluding vendor, node_modules, dist, coverage, generated. Go and TS/TSX are out of the report - Go is documented, TS/TSX may carry useful comments - but .go .ts .tsx are swept for Stub/TODO/FIXME markers." -f ($defaultPaths -join ', '), $scopeNote) `
  -Stamp $Stamp -Root $Root
Write-SanityHeader $report

function Get-CommentSpans {
  param([string]$Text, [string]$Ext)
  $spans = New-Object System.Collections.Generic.List[object]
  if ([string]::IsNullOrEmpty($Text)) { return $spans }
  $allowLine = ($Ext -ne '.css')
  $lineToken = if ($Ext -eq '.sql') { '--' } else { '//' }
  $t0 = $lineToken[0]
  $t1 = $lineToken[1]
  $chars = $Text.ToCharArray()
  $n = $chars.Length
  $line = 1
  $i = 0
  $state = 'code'
  $startLine = 0
  $kind = ''
  $sb = New-Object System.Text.StringBuilder
  while ($i -lt $n) {
    $c = $chars[$i]
    $next = if ($i + 1 -lt $n) { $chars[$i + 1] } else { [char]0 }
    switch ($state) {
      'code' {
        if ($c -eq '"') { $state = 'dq' }
        elseif ($c -eq "'") { $state = 'sq' }
        elseif ($c -eq '`') { $state = 'bt' }
        elseif ($allowLine -and $c -eq $t0 -and $next -eq $t1) {
          $prev = if ($i -gt 0) { $chars[$i - 1] } else { [char]0 }
          if ($lineToken -eq '//' -and $prev -eq ':') {
            $i++
          } else {
            $state = 'line'; $startLine = $line; $kind = 'line'; [void]$sb.Clear(); $i++
          }
        }
        elseif ($c -eq '/' -and $next -eq '*') {
          $j = $i - 1
          while ($j -ge 0 -and [char]::IsWhiteSpace($chars[$j])) { $j-- }
          $kind = 'block'
          if ($j -ge 0 -and $chars[$j] -eq '{') { $kind = 'jsx' }
          if (($i + 2) -lt $n -and $chars[$i + 2] -eq '*' -and (($i + 3) -ge $n -or $chars[$i + 3] -ne '/')) { $kind = 'doc' }
          $state = 'block'; $startLine = $line; [void]$sb.Clear(); $i++
        }
      }
      'line' {
        if ($c -eq "`n") {
          $spans.Add([pscustomobject]@{ Line = $startLine; Kind = $kind; Text = $sb.ToString().Trim() })
          $state = 'code'
        } else { [void]$sb.Append($c) }
      }
      'block' {
        if ($c -eq '*' -and $next -eq '/') {
          $spans.Add([pscustomobject]@{ Line = $startLine; Kind = $kind; Text = $sb.ToString().Trim() })
          $state = 'code'; $i++
        } else { [void]$sb.Append($c) }
      }
      'dq' {
        if ($c -eq '\' -and $Ext -ne '.sql') { $i++ }
        elseif ($c -eq '"') { $state = 'code' }
        elseif ($c -eq "`n") { $state = 'code' }
      }
      'sq' {
        if ($c -eq '\' -and $Ext -ne '.sql') { $i++ }
        elseif ($c -eq "'") { $state = 'code' }
        elseif ($c -eq "`n" -and $Ext -ne '.sql') { $state = 'code' }
      }
      'bt' {
        if ($c -eq '\' -and $Ext -ne '.go') { $i++ }
        elseif ($c -eq '`') { $state = 'code' }
      }
    }
    if ($i -lt $n -and $chars[$i] -eq "`n") { $line++ }
    $i++
  }
  if ($state -eq 'line' -or $state -eq 'block') {
    $spans.Add([pscustomobject]@{ Line = $startLine; Kind = $kind; Text = $sb.ToString().Trim() })
  }
  return $spans
}

function Write-MarkerTodo {
  param([string]$Root, [string]$TodoDir, [string]$Stamp, [object[]]$Hits, [string]$ScopeNote, [string]$FilesNote)
  $hits = @($Hits | Where-Object { $null -ne $_ })
  if ($hits.Count -eq 0) { return '' }
  if ([System.IO.Path]::IsPathRooted($TodoDir)) { $dir = $TodoDir } else { $dir = Join-Path $Root $TodoDir }
  if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
  $path = Join-Path $dir ("{0}-stub-todo-fixme.md" -f $Stamp)
  $md = New-Object System.Collections.Generic.List[string]
  $md.Add('# Stub / TODO / FIXME markers')
  $md.Add('')
  $md.Add("Generated: $((Get-Date).ToString('yyyy-MM-dd HH:mm')) by ``scripts/sanity/code-comments/check.ps1``")
  $md.Add("Scope: $ScopeNote")
  $md.Add("Markers: $($hits.Count)")
  if ($FilesNote) { $md.Add("Files swept: $FilesNote") }
  $md.Add('')
  $md.Add('Each entry is unfinished work, not a comment-style violation. Markers are')
  $md.Add('allowed; this file is the queue for closing them. Prefer "// Stub:" over "// TODO:".')
  $md.Add('Go is out of scope for the comment report but in scope here: a Go marker is debt')
  $md.Add('like any other.')
  $md.Add('')
  $md.Add('| File | Line | Marker |')
  $md.Add('|---|---|---|')
  foreach ($h in ($hits | Sort-Object -Property File, Line)) {
    $md.Add(("| ``{0}`` | {1} | {2} |" -f $h.File, $h.Line, (ConvertTo-SanityTableCell $h.Text)))
  }
  $md.Add('')
  [System.IO.File]::WriteAllLines($path, $md.ToArray(), (New-Object System.Text.UTF8Encoding($false)))
  return $path
}

function Format-CommentText {
  param([string]$Text, [string]$Kind)
  $t = ($Text -replace '\r?\n', ' ') -replace '\s+', ' '
  if ($t.Length -gt 120) { $t = $t.Substring(0, 117) + '...' }
  return ("{0}: {1}" -f $Kind, $t)
}

$scoped = [bool]($Changed -or $Since -or ($Paths -and $Paths.Count))
$goSearchPaths = if ($scoped) { $searchPaths } else { $goDefaultPaths }

$files = @()
if (@($searchPaths).Count -gt 0) {
  $files = @(Get-SanityFiles -Root $Root -Paths $searchPaths -Include $include -ExcludeMatch $excludeMatch)
}
$goFiles = @()
if (@($goSearchPaths).Count -gt 0) {
  $goFiles = @(Get-SanityFiles -Root $Root -Paths $goSearchPaths -Include $goInclude -ExcludeMatch $excludeMatch)
}

$lineHits = @(); $blockHits = @(); $docHits = @(); $jsxHits = @(); $markerHits = @(); $strayCopyright = @()
$perFile = @{}
$checked = 0

foreach ($f in $files) {
  $checked++
  $spans = Get-CommentSpans -Text (Get-SanityText $f.Full) -Ext $f.Ext
  foreach ($s in $spans) {
    $text = $s.Text
    if ($copyrightRx.IsMatch($text)) {
      if ($s.Line -le 4) { continue }
      $strayCopyright += (New-SanityHit -File $f.Rel -Line $s.Line -Text (Format-CommentText $text $s.Kind))
      continue
    }
    if ($directiveRx.IsMatch($text)) { continue }
    if ($markerRx.IsMatch($text)) {
      $markerHits += (New-SanityHit -File $f.Rel -Line $s.Line -Text (Format-CommentText $text $s.Kind))
      continue
    }
    $hit = New-SanityHit -File $f.Rel -Line $s.Line -Text (Format-CommentText $text $s.Kind)
    switch ($s.Kind) {
      'line' { $lineHits += $hit }
      'doc' { $docHits += $hit }
      'jsx' { $jsxHits += $hit }
      default { $blockHits += $hit }
    }
    if ($perFile.ContainsKey($f.Rel)) { $perFile[$f.Rel]++ } else { $perFile[$f.Rel] = 1 }
  }
}

$goChecked = 0
foreach ($f in $goFiles) {
  $goChecked++
  $goText = Get-SanityText $f.Full
  if (-not $markerPrefilterRx.IsMatch($goText)) { continue }
  foreach ($s in (Get-CommentSpans -Text $goText -Ext $f.Ext)) {
    if (-not $markerRx.IsMatch($s.Text)) { continue }
    $markerHits += (New-SanityHit -File $f.Rel -Line $s.Line -Text (Format-CommentText $s.Text $s.Kind))
  }
}

$violationMode = if ($scoped) { 'strict' } else { 'info' }

$remediation = 'Delete the comment. In CSS and SQL, unless the user asked for comments, the only ones written are the copyright header, toolchain directives and Stub/TODO/FIXME markers - see .claude/rules/code-comments.md. Pre-existing comments are left alone; scope this check with -Paths to see only your own diff.'

Add-SanityCheck -Report $report -Id 1 -Title 'Line comments that are not the copyright header' -Hits $lineHits -Mode $violationMode -Severity low `
  -Remediation $remediation -Note ("{0} files tokenised." -f $checked)
Add-SanityCheck -Report $report -Id 2 -Title 'Block comments that are not the copyright header' -Hits $blockHits -Mode $violationMode -Severity low -Remediation $remediation
Add-SanityCheck -Report $report -Id 3 -Title 'Doc comments (/** ... */)' -Hits $docHits -Mode $violationMode -Severity low -Remediation $remediation
Add-SanityCheck -Report $report -Id 4 -Title 'JSX comments ({/* ... */})' -Hits $jsxHits -Mode $violationMode -Severity low -Remediation $remediation
Add-SanityCheck -Report $report -Id 5 -Title 'Copyright text outside the two-line header' -Hits $strayCopyright -Mode review -Severity low `
  -Remediation 'The header belongs on the first two lines, after any shebang. A third-party notice further down is acceptable - record it here.'

$top = @($perFile.GetEnumerator() | Sort-Object -Property Value -Descending | Select-Object -First 40 | ForEach-Object { , @($_.Key, $_.Value) })
if ($top.Count -gt 0) {
  Add-SanityCheck -Report $report -Id 6 -Title 'Comment density by file (inventory table below)' -Hits @() -Mode info -Severity info `
    -Table @{ Headers = @('File', 'Comments'); Rows = $top }
}

$filesNote = ("{0} CSS/SQL, {1} Go/TS/TSX" -f $checked, $goChecked)
$todoPath = Write-MarkerTodo -Root $Root -TodoDir $TodoDir -Stamp $Stamp -Hits $markerHits -ScopeNote $scopeNote -FilesNote $filesNote
if ($todoPath) {
  Add-SanityNote -Report $report -Text ("{0} Stub/TODO/FIXME marker(s) - Go and TS/TSX included - written to ``{1}``; they are code-fix debt, not a sanity finding." -f @($markerHits).Count, (Get-SanityRelPath -Root $Root -Full $todoPath))
}

if (-not $todoPath) {
  Add-SanityNote -Report $report -Text ("0 Stub/TODO/FIXME markers found in {0} .go/.ts/.tsx file(s) swept; no file is written under {1} when there is nothing to queue." -f $goChecked, $TodoDir)
}

Add-SanityNote -Report $report -Text 'Files are tokenised (string and template-literal aware), so a "//" inside a string or a URL is not reported.'
Add-SanityNote -Report $report -Text ('Go and TS/TSX are deliberately out of scope for checks 1-6: .go files are documented to the Go doc-comment standard (go-standards), and TS/TSX may carry useful comments whose value is left to code review. {0} .go/.ts/.tsx file(s) were still swept for Stub/TODO/FIXME markers, which land in the code-fix queue.' -f $goChecked)
Add-SanityNote -Report $report -Text ('Checks 1-4 are {0} in this run. A full-tree run is inventory only (the rule leaves pre-existing comments alone); scoping with -Changed, -Since <ref> or -Paths makes them a pass/fail gate on your own diff.' -f $violationMode)
Add-SanityNote -Report $report -Text 'Rule: .claude/rules/code-comments.md. Instructions: .instructions/sanity/code-comments.md.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
