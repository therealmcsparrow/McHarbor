# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Shared helpers for scripts/sanity/<check>/check.ps1.
#
# Every check dot-sources this file:
#   . (Join-Path $PSScriptRoot '..\_lib\common.ps1')
#
# Design notes
# - Pure PowerShell 5.1. No ripgrep, no external modules. Regexes are .NET.
# - File trees are walked once per (path, exclude set) and cached; file
#   contents are cached per run. A check that issues 30 regex sweeps still
#   reads each file exactly once.
# - Checks never modify project files. They only write reports under
#   .results/sanity/ (or -OutDir).

Set-StrictMode -Off

$script:SanityDefaultExcludeDirs = @('node_modules', 'dist', 'vendor', 'coverage', '.git', 'build', '.unused', 'output', '.ladle', 'playwright-report', 'test-results')
$script:SanityTreeCache = @{}
$script:SanityLineCache = @{}
$script:SanityTextCache = @{}

# JSX tag body: everything up to the closing '>' of the opening tag, tolerating
# arrow functions (=>) inside prop values. Use as: "<Button\b$SanityJsxTagBody>"
$script:SanityJsxTagBody = '(?:=>|[^>])*'

# database/sql call sites; excludes r.URL.Query() and other non-DB .Query( uses
$script:SanityDbCallPattern = '\.(Query|QueryRow|Exec)(Context)?\((?!\))'
$script:SanityDbCallNotMatch = 'URL\.Query\(|\.Query\(\)|url\.Values|\bq\.Get\(|RawQuery|apispec\.Query\('

# ---------------------------------------------------------------------------
# Paths and environment
# ---------------------------------------------------------------------------

function Get-SanityRoot {
  param([string]$Root, [string]$ScriptDir)
  if ([string]::IsNullOrWhiteSpace($Root)) {
    $Root = Join-Path $ScriptDir '..\..\..'
  }
  return (Resolve-Path -LiteralPath $Root).Path
}

function Get-SanityStamp {
  return (Get-Date -Format 'yyyy-MM-dd-HHmm')
}

function Get-SanityOutDir {
  param([string]$Root, [string]$OutDir)
  if ([System.IO.Path]::IsPathRooted($OutDir)) { $full = $OutDir } else { $full = Join-Path $Root $OutDir }
  if (-not (Test-Path -LiteralPath $full)) { New-Item -ItemType Directory -Force -Path $full | Out-Null }
  return (Resolve-Path -LiteralPath $full).Path
}

function Get-SanityRelPath {
  param([string]$Root, [string]$Full)
  $r = $Root.TrimEnd('\', '/')
  if ($Full.StartsWith($r, [System.StringComparison]::OrdinalIgnoreCase)) {
    $rel = $Full.Substring($r.Length).TrimStart('\', '/')
  } else {
    $rel = $Full
  }
  return ($rel -replace '\\', '/')
}

function Get-SanityScope {
  param(
    [Parameter(Mandatory)][string]$Root,
    [string[]]$Paths,
    [string[]]$Default = @()
  )
  if (-not $Paths -or $Paths.Count -eq 0) { return $Default }
  $flat = New-Object System.Collections.Generic.List[string]
  foreach ($p in $Paths) {
    if ([string]::IsNullOrWhiteSpace($p)) { continue }
    foreach ($part in ($p -split ',')) {
      $t = $part.Trim()
      if ($t) { $flat.Add($t) }
    }
  }
  if ($flat.Count -eq 0) { return $Default }
  $rootNorm = ($Root -replace '\\', '/').TrimEnd('/')
  $defaultsNorm = @($Default | ForEach-Object { ($_ -replace '\\', '/').Trim('/') } | Where-Object { $_ })
  $out = New-Object System.Collections.Generic.List[string]
  foreach ($p in $flat) {
    $norm = ($p -replace '\\', '/')
    if ([System.IO.Path]::IsPathRooted($p)) {
      if ($norm.ToLowerInvariant().StartsWith($rootNorm.ToLowerInvariant() + '/')) {
        $norm = $norm.Substring($rootNorm.Length).TrimStart('/')
      } else {
        continue
      }
    }
    $norm = $norm.TrimStart('/')
    if (-not $norm) { continue }
    $normLower = $norm.ToLowerInvariant()
    foreach ($d in $defaultsNorm) {
      $dLower = $d.ToLowerInvariant()
      if ($normLower -eq $dLower) { $out.Add($d) }
      elseif ($normLower.StartsWith($dLower + '/')) { $out.Add($norm) }
      elseif ($dLower.StartsWith($normLower + '/')) { $out.Add($d) }
    }
  }
  if ($out.Count -eq 0) { return @() }
  $unique = @($out.ToArray() | Sort-Object -Unique)
  $result = New-Object System.Collections.Generic.List[string]
  foreach ($a in $unique) {
    $aLower = $a.ToLowerInvariant()
    $subsumed = $false
    foreach ($b in $unique) {
      if ($a -eq $b) { continue }
      if ($aLower.StartsWith($b.ToLowerInvariant() + '/')) { $subsumed = $true; break }
    }
    if (-not $subsumed) { $result.Add($a) }
  }
  return @($result.ToArray() | Sort-Object -Unique)
}

function Get-SanityPluginPaths {
  param([Parameter(Mandatory)][string]$Root, [Parameter(Mandatory)][string]$SubPath)
  $pluginRoot = Join-Path $Root 'src\plugins\Plugins'
  if (-not (Test-Path -LiteralPath $pluginRoot)) { return @() }
  $out = New-Object System.Collections.Generic.List[string]
  foreach ($d in (Get-ChildItem -LiteralPath $pluginRoot -Directory -ErrorAction SilentlyContinue)) {
    $candidate = Join-Path $d.FullName $SubPath
    if (Test-Path -LiteralPath $candidate) { $out.Add("src/plugins/Plugins/$($d.Name)/$($SubPath -replace '\\', '/')") }
  }
  return $out.ToArray()
}

function Resolve-SanityTool {
  param([string]$Name)
  # On Windows, Get-Command may resolve 'npm' to the bash shim (extensionless);
  # Process.Start cannot launch that, so prefer the .cmd / .exe / .bat variant.
  $candidates = @($Name)
  if ($env:OS -eq 'Windows_NT' -and $Name -notmatch '\.(cmd|exe|bat|ps1)$') { $candidates = @("$Name.cmd", "$Name.exe", "$Name.bat", $Name) }
  foreach ($c in $candidates) {
    $cmd = Get-Command $c -ErrorAction SilentlyContinue | Where-Object { $_.CommandType -eq 'Application' } | Select-Object -First 1
    if ($null -eq $cmd) { continue }
    if ([System.IO.Path]::GetExtension($cmd.Source) -eq '' -and $env:OS -eq 'Windows_NT') { continue }
    return $cmd.Source
  }
  $cmd = Get-Command $Name -ErrorAction SilentlyContinue
  if ($null -eq $cmd) { return $null }
  if ($cmd.CommandType -eq 'Application') { return $cmd.Source }
  return $cmd.Name
}

# ---------------------------------------------------------------------------
# File discovery and cached reads
# ---------------------------------------------------------------------------

function Get-SanityTreeFiles {
  param([string]$Dir, [string[]]$ExcludeDirs, [System.Collections.Generic.List[string]]$Acc)
  try { $files = [System.IO.Directory]::GetFiles($Dir) } catch { $files = @() }
  foreach ($f in $files) { $Acc.Add($f) }
  try { $dirs = [System.IO.Directory]::GetDirectories($Dir) } catch { $dirs = @() }
  foreach ($d in $dirs) {
    $name = [System.IO.Path]::GetFileName($d)
    if ($ExcludeDirs -contains $name) { continue }
    Get-SanityTreeFiles -Dir $d -ExcludeDirs $ExcludeDirs -Acc $Acc
  }
}

function Test-SanityGlob {
  param([string]$Name, [string[]]$Globs)
  foreach ($g in $Globs) { if ($Name -like $g) { return $true } }
  return $false
}

# Returns objects: Full, Rel (posix, relative to Root), Name, Ext, Dir (posix rel dir)
function Get-SanityFiles {
  param(
    [Parameter(Mandatory)][string]$Root,
    [string[]]$Paths = @('src'),
    [string[]]$Include = @('*'),
    [string[]]$ExcludeDirs = $script:SanityDefaultExcludeDirs,
    [string[]]$ExcludeMatch = @()
  )
  $out = New-Object System.Collections.Generic.List[object]
  foreach ($p in $Paths) {
    if ([System.IO.Path]::IsPathRooted($p)) { $full = $p } else { $full = Join-Path $Root $p }
    if (-not (Test-Path -LiteralPath $full)) { continue }
    $item = Get-Item -LiteralPath $full
    if ($item.PSIsContainer) {
      $key = $item.FullName.ToLowerInvariant() + '|' + ($ExcludeDirs -join ',')
      if (-not $script:SanityTreeCache.ContainsKey($key)) {
        $acc = New-Object System.Collections.Generic.List[string]
        Get-SanityTreeFiles -Dir $item.FullName -ExcludeDirs $ExcludeDirs -Acc $acc
        $script:SanityTreeCache[$key] = $acc.ToArray()
      }
      $candidates = $script:SanityTreeCache[$key]
    } else {
      $candidates = @($item.FullName)
    }
    foreach ($c in $candidates) {
      $name = [System.IO.Path]::GetFileName($c)
      if (-not (Test-SanityGlob -Name $name -Globs $Include)) { continue }
      $rel = Get-SanityRelPath -Root $Root -Full $c
      $skip = $false
      foreach ($x in $ExcludeMatch) { if ($rel -match $x) { $skip = $true; break } }
      if ($skip) { continue }
      $out.Add([pscustomobject]@{
        Full = $c
        Rel  = $rel
        Name = $name
        Ext  = [System.IO.Path]::GetExtension($c).ToLowerInvariant()
        Dir  = (($rel -replace '/[^/]*$', '') -replace '^[^/]*$', '')
      })
    }
  }
  return $out.ToArray()
}

function Get-SanityLines {
  param([string]$Full)
  if (-not $script:SanityLineCache.ContainsKey($Full)) {
    try { $script:SanityLineCache[$Full] = [System.IO.File]::ReadAllLines($Full) } catch { $script:SanityLineCache[$Full] = @() }
  }
  return $script:SanityLineCache[$Full]
}

function Get-SanityText {
  param([string]$Full)
  if (-not $script:SanityTextCache.ContainsKey($Full)) {
    try { $script:SanityTextCache[$Full] = [System.IO.File]::ReadAllText($Full) } catch { $script:SanityTextCache[$Full] = '' }
  }
  return $script:SanityTextCache[$Full]
}

function Get-SanityLineNumber {
  param([string]$Text, [int]$Index)
  if ($Index -le 0) { return 1 }
  $n = 1
  $limit = [Math]::Min($Index, $Text.Length)
  for ($i = 0; $i -lt $limit; $i++) { if ($Text[$i] -eq "`n") { $n++ } }
  return $n
}

function Remove-SanityStringLiterals {
  param([string]$Line)
  $l = [regex]::Replace($Line, '"(?:[^"\\]|\\.)*"', '""')
  $l = [regex]::Replace($l, '`[^`]*`', '``')
  # A line that opens a Go raw string carries code up to the backtick and
  # string content after it. Without this the opening line of every embedded
  # SQL statement is scanned as if it were code.
  $tick = $l.IndexOf('`')
  if ($tick -ge 0) { $l = $l.Substring(0, $tick) }
  return $l
}

# Remove-SanityStringLiterals only sees one line, so a Go raw string that spans
# lines - a SQL statement, an install script, an OpenAPI blob - keeps matching
# every naming and style pattern in the suite. Those lines are data, not code.
$script:SanityRawStringCache = @{}
function Get-SanityRawStringLines {
  param([string]$Full)
  if ($script:SanityRawStringCache.ContainsKey($Full)) { return $script:SanityRawStringCache[$Full] }
  # A hashtable, not a HashSet: PowerShell unrolls an enumerable return value,
  # so an empty set comes back as $null and every caller has to guard for it.
  $set = @{}
  $lines = Get-SanityLines $Full
  $inRaw = $false
  for ($i = 0; $i -lt $lines.Length; $i++) {
    if ($inRaw) { $set[($i + 1)] = $true }
    $ticks = ([regex]::Matches($lines[$i], '`')).Count
    if ($ticks % 2 -eq 1) { $inRaw = -not $inRaw }
  }
  $script:SanityRawStringCache[$Full] = $set
  return $set
}

# Drops hits whose line sits inside a multi-line raw string literal.
function Remove-SanityRawStringHits {
  param([object[]]$Hits, [string]$Root)
  return @($Hits | Where-Object {
      $full = Join-Path $Root ($_.File -replace '/', [System.IO.Path]::DirectorySeparatorChar)
      if (-not (Test-Path -LiteralPath $full)) { return $true }
      -not (Get-SanityRawStringLines $full).ContainsKey([int]$_.Line)
    })
}

function New-SanityHit {
  param([string]$File, [int]$Line = 0, [string]$Text = '')
  if ($Text.Length -gt 220) { $Text = $Text.Substring(0, 217) + '...' }
  return [pscustomobject]@{ File = $File; Line = $Line; Text = $Text }
}

# ---------------------------------------------------------------------------
# Search
# ---------------------------------------------------------------------------

# Line-oriented (default) or whole-file (-Multiline) regex search.
# Returns hit objects {File, Line, Text} (+ Match for -Multiline).
function Find-Pattern {
  param(
    [Parameter(Mandatory)][string]$Root,
    [Parameter(Mandatory)][string]$Pattern,
    [string[]]$Paths = @('src'),
    [string[]]$Include = @('*'),
    [string[]]$ExcludeDirs = $script:SanityDefaultExcludeDirs,
    [string[]]$ExcludeMatch = @(),
    [switch]$IgnoreCase,
    [switch]$Multiline,
    [switch]$StripStrings,
    [string]$NotMatch = '',
    [string]$AlsoMatch = ''
  )
  $opts = [System.Text.RegularExpressions.RegexOptions]::None
  if ($IgnoreCase) { $opts = $opts -bor [System.Text.RegularExpressions.RegexOptions]::IgnoreCase }
  if ($Multiline) { $opts = $opts -bor [System.Text.RegularExpressions.RegexOptions]::Multiline }
  $rx = New-Object System.Text.RegularExpressions.Regex($Pattern, $opts)
  $notRx = $null; if ($NotMatch) { $notRx = New-Object System.Text.RegularExpressions.Regex($NotMatch) }
  $alsoRx = $null; if ($AlsoMatch) { $alsoRx = New-Object System.Text.RegularExpressions.Regex($AlsoMatch) }
  $results = New-Object System.Collections.Generic.List[object]
  $files = Get-SanityFiles -Root $Root -Paths $Paths -Include $Include -ExcludeDirs $ExcludeDirs -ExcludeMatch $ExcludeMatch
  foreach ($f in $files) {
    if ($Multiline) {
      $text = Get-SanityText $f.Full
      if ([string]::IsNullOrEmpty($text)) { continue }
      foreach ($m in $rx.Matches($text)) {
        $val = $m.Value
        if ($notRx -and $notRx.IsMatch($val)) { continue }
        if ($alsoRx -and -not $alsoRx.IsMatch($val)) { continue }
        $first = ($val -split "`r?`n")[0].Trim()
        $lineNo = Get-SanityLineNumber -Text $text -Index $m.Index
        $lineStart = $text.LastIndexOf("`n", [Math]::Max(0, $m.Index - 1)) + 1
        $lineEnd = $text.IndexOf("`n", $m.Index); if ($lineEnd -lt 0) { $lineEnd = $text.Length }
        $lineText = $text.Substring($lineStart, $lineEnd - $lineStart).TrimEnd("`r")
        if ($lineText -match '^\s*(//|\*|/\*|\{/\*|#)') { continue }   # match starts inside a comment line
        $hit = New-SanityHit -File $f.Rel -Line $lineNo -Text $first
        $hit | Add-Member -NotePropertyName Match -NotePropertyValue $val
        $hit | Add-Member -NotePropertyName LineText -NotePropertyValue $lineText
        $results.Add($hit)
      }
    } else {
      $lines = Get-SanityLines $f.Full
      for ($i = 0; $i -lt $lines.Length; $i++) {
        $probe = $lines[$i]
        if ($StripStrings) { $probe = Remove-SanityStringLiterals $probe }
        if (-not $rx.IsMatch($probe)) { continue }
        if ($notRx -and $notRx.IsMatch($lines[$i])) { continue }
        if ($alsoRx -and -not $alsoRx.IsMatch($lines[$i])) { continue }
        $results.Add((New-SanityHit -File $f.Rel -Line ($i + 1) -Text $lines[$i].Trim()))
      }
    }
  }
  return $results.ToArray()
}

# Files whose content does NOT match the pattern (rg -L equivalent).
function Find-FilesWithoutMatch {
  param(
    [Parameter(Mandatory)][string]$Root,
    [Parameter(Mandatory)][string]$Pattern,
    [string[]]$Paths = @('src'),
    [string[]]$Include = @('*'),
    [string[]]$ExcludeDirs = $script:SanityDefaultExcludeDirs,
    [string[]]$ExcludeMatch = @(),
    [string]$OnlyIfMatch = ''
  )
  $rx = New-Object System.Text.RegularExpressions.Regex($Pattern)
  $onlyRx = $null; if ($OnlyIfMatch) { $onlyRx = New-Object System.Text.RegularExpressions.Regex($OnlyIfMatch) }
  $results = New-Object System.Collections.Generic.List[object]
  foreach ($f in (Get-SanityFiles -Root $Root -Paths $Paths -Include $Include -ExcludeDirs $ExcludeDirs -ExcludeMatch $ExcludeMatch)) {
    $text = Get-SanityText $f.Full
    if ($onlyRx -and -not $onlyRx.IsMatch($text)) { continue }
    if (-not $rx.IsMatch($text)) { $results.Add((New-SanityHit -File $f.Rel -Line 0 -Text "no match for /$Pattern/")) }
  }
  return $results.ToArray()
}

# Identifier frequency table over a set of files (fast "how many times is X
# referenced anywhere" lookups without O(names x corpus) scans).
function Get-SanityTokenCounts {
  param([object[]]$Files, [string]$TokenPattern = '[A-Za-z_][A-Za-z0-9_]*')
  $counts = New-Object 'System.Collections.Generic.Dictionary[string,int]'
  $rx = New-Object System.Text.RegularExpressions.Regex($TokenPattern)
  foreach ($f in $Files) {
    $text = Get-SanityText $f.Full
    if ([string]::IsNullOrEmpty($text)) { continue }
    foreach ($m in $rx.Matches($text)) {
      $t = $m.Value
      if ($counts.ContainsKey($t)) { $counts[$t]++ } else { $counts[$t] = 1 }
    }
  }
  return $counts
}

# ---------------------------------------------------------------------------
# JSON (JavaScriptSerializer keeps key case and handles large files)
# ---------------------------------------------------------------------------

function ConvertFrom-SanityJson {
  param([string]$Text)
  try {
    Add-Type -AssemblyName System.Web.Extensions -ErrorAction Stop
    $ser = New-Object System.Web.Script.Serialization.JavaScriptSerializer
    $ser.MaxJsonLength = [int]::MaxValue
    $ser.RecursionLimit = 512
    return $ser.DeserializeObject($Text)
  } catch {
    return ($Text | ConvertFrom-Json)
  }
}

# Returns the value at $Key. Arrays are returned wrapped so an empty array does
# not collapse to $null on the way out of the function (a PowerShell pipeline trap
# that made "field: []" look like a missing field).
function Get-SanityJsonValue {
  param($Obj, [string]$Key)
  if ($null -eq $Obj) { return $null }
  if ($Obj -is [System.Collections.IDictionary]) {
    $has = $false
    $m = $Obj.GetType().GetMethod('ContainsKey')
    if ($null -ne $m) { $has = [bool]$m.Invoke($Obj, @($Key)) } else { $has = $Obj.Contains([object]$Key) }
    if (-not $has) { return $null }
    $v = $Obj[$Key]
    if ($v -is [System.Array]) { return ,$v }
    return $v
  }
  $p = $Obj.PSObject.Properties[$Key]
  if ($null -ne $p) { if ($p.Value -is [System.Array]) { return ,$p.Value } else { return $p.Value } }
  return $null
}

# Returns the value at $Key as a flat array of elements (empty when absent or
# empty). Use this instead of @(Get-SanityJsonValue ...) for JSON arrays.
function Get-SanityJsonArray {
  param($Obj, [string]$Key)
  $v = Get-SanityJsonValue $Obj $Key
  if ($null -eq $v) { return @() }
  if ($v -is [System.Array]) { return $v }
  if ($v -is [System.Collections.IEnumerable] -and -not ($v -is [string])) { return @($v) }
  return @($v)
}

# Flattens a nested dictionary into dot-notation keys (leaves only).
function Get-SanityFlatKeys {
  param($Obj, [string]$Prefix = '', [System.Collections.Generic.List[string]]$Acc)
  if ($null -eq $Acc) { $Acc = New-Object System.Collections.Generic.List[string] }
  if ($Obj -is [System.Collections.IDictionary]) {
    foreach ($k in $Obj.Keys) {
      $child = $Obj[$k]
      $path = if ($Prefix) { "$Prefix.$k" } else { "$k" }
      if ($child -is [System.Collections.IDictionary]) { Get-SanityFlatKeys -Obj $child -Prefix $path -Acc $Acc | Out-Null }
      else { $Acc.Add($path) }
    }
  } elseif ($Obj -is [System.Management.Automation.PSCustomObject]) {
    foreach ($p in $Obj.PSObject.Properties) {
      $path = if ($Prefix) { "$Prefix.$($p.Name)" } else { $p.Name }
      if ($p.Value -is [System.Management.Automation.PSCustomObject] -or $p.Value -is [System.Collections.IDictionary]) { Get-SanityFlatKeys -Obj $p.Value -Prefix $path -Acc $Acc | Out-Null }
      else { $Acc.Add($path) }
    }
  }
  return $Acc
}

# ---------------------------------------------------------------------------
# External commands (npm, go) with timeout and captured output
# ---------------------------------------------------------------------------

function Invoke-SanityCommand {
  param(
    [Parameter(Mandatory)][string]$FilePath,
    [string]$Arguments = '',
    [string]$WorkingDirectory = '',
    [int]$TimeoutSec = 300
  )
  $psi = New-Object System.Diagnostics.ProcessStartInfo
  $psi.FileName = $FilePath
  $psi.Arguments = $Arguments
  if ($WorkingDirectory) { $psi.WorkingDirectory = $WorkingDirectory }
  $psi.RedirectStandardOutput = $true
  $psi.RedirectStandardError = $true
  $psi.UseShellExecute = $false
  $psi.CreateNoWindow = $true
  try {
    $p = [System.Diagnostics.Process]::Start($psi)
  } catch {
    return [pscustomobject]@{ ExitCode = -1; StdOut = ''; StdErr = "$($_.Exception.Message)"; TimedOut = $false; Failed = $true }
  }
  $outTask = $p.StandardOutput.ReadToEndAsync()
  $errTask = $p.StandardError.ReadToEndAsync()
  if (-not $p.WaitForExit($TimeoutSec * 1000)) {
    try { $p.Kill() } catch { }
    return [pscustomobject]@{ ExitCode = -1; StdOut = ''; StdErr = "timed out after ${TimeoutSec}s"; TimedOut = $true; Failed = $true }
  }
  $p.WaitForExit()
  return [pscustomobject]@{ ExitCode = $p.ExitCode; StdOut = $outTask.Result; StdErr = $errTask.Result; TimedOut = $false; Failed = $false }
}

# ---------------------------------------------------------------------------
# Report model
# ---------------------------------------------------------------------------

function New-SanityReport {
  param(
    [Parameter(Mandatory)][string]$Name,
    [string]$Title = '',
    [string]$Scope = '',
    [string]$Stamp = '',
    [string]$Root = '',
    [string]$Source = ''
  )
  if (-not $Title) { $Title = "$Name sanity" }
  if (-not $Stamp) { $Stamp = Get-SanityStamp }
  if (-not $Source) { $Source = ".instructions/sanity/$Name.md" }
  return [pscustomobject]@{
    Name    = $Name
    Title   = $Title
    Scope   = $Scope
    Stamp   = $Stamp
    Root    = $Root
    Source  = $Source
    Checks  = (New-Object System.Collections.Generic.List[object])
    Notes   = (New-Object System.Collections.Generic.List[string])
    Started = (Get-Date)
  }
}

function Write-SanityHeader {
  param($Report)
  Write-Host ''
  Write-Host ("== {0} ==" -f $Report.Title) -ForegroundColor White
  Write-Host ("   scope: {0}" -f $Report.Scope) -ForegroundColor DarkGray
}

function Get-SanityVerdictColor {
  param([string]$Verdict)
  switch ($Verdict) {
    'pass'    { return 'Green' }
    'fail'    { return 'Red' }
    'review'  { return 'Yellow' }
    'info'    { return 'Cyan' }
    'skipped' { return 'DarkGray' }
    default   { return 'Gray' }
  }
}

# Mode semantics
#   strict : any hit is a violation                    -> fail
#   review : hits need a human to confirm/dismiss      -> review
#   info   : hits are inventory / context only         -> info
function Add-SanityCheck {
  param(
    [Parameter(Mandatory)]$Report,
    [Parameter(Mandatory)][string]$Id,
    [Parameter(Mandatory)][string]$Title,
    [object[]]$Hits = @(),
    [ValidateSet('strict', 'review', 'info')][string]$Mode = 'review',
    [ValidateSet('high', 'medium', 'low', 'info')][string]$Severity = 'medium',
    [string]$Remediation = '',
    [string]$Note = '',
    [string]$Command = '',
    [switch]$Skipped,
    [string]$SkipReason = '',
    [hashtable]$Table = $null
  )
  $hits = @($Hits | Where-Object { $null -ne $_ })
  $count = $hits.Count
  if ($Skipped) { $verdict = 'skipped' }
  elseif ($count -eq 0) { $verdict = 'pass' }
  elseif ($Mode -eq 'strict') { $verdict = 'fail' }
  elseif ($Mode -eq 'info') { $verdict = 'info' }
  else { $verdict = 'review' }
  $Report.Checks.Add([pscustomobject]@{
    Id = $Id; Title = $Title; Hits = $hits; Count = $count; Mode = $Mode; Severity = $Severity
    Verdict = $verdict; Remediation = $Remediation; Note = $Note; Command = $Command; SkipReason = $SkipReason; Table = $Table
  })
  $label = $verdict.ToUpperInvariant().PadRight(7)
  $suffix = if ($Skipped) { "skipped: $SkipReason" } else { "$count hit(s)" }
  Write-Host ("   [{0}] {1,3}. {2} - {3}" -f $label, $Id, $Title, $suffix) -ForegroundColor (Get-SanityVerdictColor $verdict)
}

function Set-SanitySetupBroken {
  param([Parameter(Mandatory)]$Report, [Parameter(Mandatory)][string]$Reason)
  $Report | Add-Member -NotePropertyName SanitySetupBroken -NotePropertyValue $true -Force
  $Report | Add-Member -NotePropertyName SanitySetupBrokenReason -NotePropertyValue $Reason -Force
}

function Test-SanitySetupBroken {
  param([Parameter(Mandatory)]$Report)
  return [bool]($Report.PSObject.Properties['SanitySetupBroken'] -and $Report.SanitySetupBroken)
}

function Add-SanityNote {
  param([Parameter(Mandatory)]$Report, [Parameter(Mandatory)][string]$Text)
  $Report.Notes.Add($Text)
}

function Format-SanityHit {
  param($Hit)
  if ($Hit -is [string]) { return $Hit }
  $file = Get-SanityJsonValue $Hit 'File'
  $line = Get-SanityJsonValue $Hit 'Line'
  $text = Get-SanityJsonValue $Hit 'Text'
  if ($null -eq $file) { return "$Hit" }
  $loc = if ($line -and $line -gt 0) { "``$file`:$line``" } else { "``$file``" }
  if ([string]::IsNullOrWhiteSpace("$text")) { return $loc }
  $clean = ("$text" -replace '\r?\n', ' ') -replace '`', "'"
  return "$loc - $clean"
}

function ConvertTo-SanityTableCell {
  param([string]$Value)
  return (("$Value" -replace '\r?\n', ' ') -replace '\|', '\|')
}

function Write-SanityReport {
  param(
    [Parameter(Mandatory)]$Report,
    [Parameter(Mandatory)][string]$OutDir,
    [int]$MaxHits = 150
  )
  $path = Join-Path $OutDir ("{0}-{1}.md" -f $Report.Stamp, $Report.Name)
  $md = New-Object System.Collections.Generic.List[string]
  $md.Add("# $($Report.Title)")
  $md.Add('')
  $md.Add("Date: $($Report.Started.ToString('yyyy-MM-dd HH:mm'))")
  $md.Add("Scope: $($Report.Scope)")
  $md.Add("Root: ``$($Report.Root)``")
  $md.Add("Instructions: ``$($Report.Source)``")
  $md.Add("Method: ``scripts/sanity/$($Report.Name)/check.ps1`` (pure PowerShell regex sweep, no ripgrep required)")
  $md.Add('')
  $md.Add('Verdicts: `pass` = no hits, `fail` = unambiguous violation, `review` = hits that need a human to confirm or dismiss, `info` = inventory only, `skipped` = prerequisite missing.')
  $md.Add('')
  $md.Add('## Summary')
  $md.Add('')
  $md.Add('| # | Check | Severity | Hits | Verdict |')
  $md.Add('|---|---|---|---|---|')
  foreach ($c in $Report.Checks) {
    $md.Add(("| {0} | {1} | {2} | {3} | **{4}** |" -f $c.Id, (ConvertTo-SanityTableCell $c.Title), $c.Severity, $c.Count, $c.Verdict))
  }
  $md.Add('')
  $md.Add('## Findings')
  $md.Add('')
  foreach ($c in $Report.Checks) {
    $md.Add(("### {0}. {1} - {2} ({3} hits)" -f $c.Id, $c.Title, $c.Verdict.ToUpperInvariant(), $c.Count))
    $md.Add('')
    if ($c.Command) { $md.Add("Command: ``$($c.Command)``"); $md.Add('') }
    if ($c.Verdict -eq 'skipped') { $md.Add("Skipped: $($c.SkipReason)"); $md.Add(''); continue }
    if ($c.Remediation) { $md.Add("Remediation: $($c.Remediation)"); $md.Add('') }
    if ($c.Note) { $md.Add("Note: $($c.Note)"); $md.Add('') }
    if ($c.Table -and $c.Table.Rows -and @($c.Table.Rows).Count -gt 0) {
      $md.Add('| ' + (($c.Table.Headers | ForEach-Object { ConvertTo-SanityTableCell $_ }) -join ' | ') + ' |')
      $md.Add('|' + (($c.Table.Headers | ForEach-Object { '---' }) -join '|') + '|')
      $shown = 0
      foreach ($row in $c.Table.Rows) {
        if ($shown -ge $MaxHits) { break }
        $md.Add('| ' + (($row | ForEach-Object { ConvertTo-SanityTableCell $_ }) -join ' | ') + ' |')
        $shown++
      }
      if (@($c.Table.Rows).Count -gt $shown) { $md.Add(''); $md.Add("_Showing $shown of $(@($c.Table.Rows).Count) rows._") }
      $md.Add('')
    }
    if ($c.Count -eq 0) {
      $md.Add("OK: no hits for this check.")
    } else {
      $shown = 0
      foreach ($h in $c.Hits) {
        if ($shown -ge $MaxHits) { break }
        $md.Add('- ' + (Format-SanityHit $h))
        $shown++
      }
      if ($c.Count -gt $shown) { $md.Add(''); $md.Add("_Showing $shown of $($c.Count) hits. Raise -MaxHits to list more._") }
    }
    $md.Add('')
  }
  $md.Add('## Notes')
  $md.Add('')
  if ($Report.Notes.Count -eq 0) { $md.Add('- none') } else { foreach ($n in $Report.Notes) { $md.Add("- $n") } }
  $md.Add('')
  [System.IO.File]::WriteAllLines($path, $md.ToArray(), (New-Object System.Text.UTF8Encoding($false)))
  return $path
}

function Get-SanitySummary {
  param([Parameter(Mandatory)]$Report, [string]$ReportPath = '')
  $fail = @($Report.Checks | Where-Object { $_.Verdict -eq 'fail' }).Count
  $review = @($Report.Checks | Where-Object { $_.Verdict -eq 'review' }).Count
  $info = @($Report.Checks | Where-Object { $_.Verdict -eq 'info' }).Count
  $pass = @($Report.Checks | Where-Object { $_.Verdict -eq 'pass' }).Count
  $skipped = @($Report.Checks | Where-Object { $_.Verdict -eq 'skipped' }).Count
  $status = if ($fail -gt 0) { 'fail' } elseif ($review -gt 0) { 'review' } else { 'pass' }
  $hits = 0; foreach ($c in $Report.Checks) { $hits += $c.Count }
  return [pscustomobject]@{
    Check   = $Report.Name
    Status  = $status
    Fail    = $fail
    Review  = $review
    Info    = $info
    Pass    = $pass
    Skipped = $skipped
    Hits    = $hits
    Seconds = [Math]::Round(((Get-Date) - $Report.Started).TotalSeconds, 1)
    Report  = $ReportPath
  }
}

function Complete-SanityCheck {
  param(
    [Parameter(Mandatory)]$Report,
    [Parameter(Mandatory)][string]$OutDir,
    [int]$MaxHits = 150
  )
  $dir = Get-SanityOutDir -Root $Report.Root -OutDir $OutDir
  $path = Write-SanityReport -Report $Report -OutDir $dir -MaxHits $MaxHits
  $summary = Get-SanitySummary -Report $Report -ReportPath $path
  Write-Host ("   -> {0}  (fail={1} review={2} info={3} pass={4} skipped={5}, {6}s)" -f $summary.Status.ToUpperInvariant(), $summary.Fail, $summary.Review, $summary.Info, $summary.Pass, $summary.Skipped, $summary.Seconds) -ForegroundColor (Get-SanityVerdictColor $summary.Status)
  Write-Host ("   report: {0}" -f (Get-SanityRelPath -Root $Report.Root -Full $path)) -ForegroundColor DarkGray
  return $summary
}
