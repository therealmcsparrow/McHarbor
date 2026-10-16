# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Performance sanity check. Mirrors .instructions/sanity/performance.md.
.EXAMPLE
  .\scripts\sanity\performance\check.ps1
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

$report = New-SanityReport -Name 'performance' -Title 'Performance Sanity' -Scope 'src/backend, src/frontend' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$goPaths = @('src/backend/modules', 'src/backend/core')
$goFiles = @(Get-SanityFiles -Root $Root -Paths $goPaths -Include @('*.go') -ExcludeMatch @('_test\.go$'))

# 1. N+1 queries: a DB call inside a range loop / rows.Next loop
#
# Reads and writes are reported separately. A read per item is the N+1 shape
# worth fixing: it multiplies round-trips to answer one question, and it
# batches into a single IN (...) statement. A write per item inside a
# transaction is the intended idiom on SQLite — there is no multi-row UPDATE
# to collapse it into — so it is an inventory, not a finding.
#
# A prepared statement executed in a loop is excluded from both: `Prepare`
# outside the loop plus `stmt.Exec` inside it *is* the batching advice this
# check would otherwise give.
$nplus1Read = @()
$nplus1Write = @()
$nplus1PoolWrite = @()
# Files that cannot carry a marker: core/db is linked into every plugin .so, so even a
# comment changes its build ID and unloads every installed plugin. Each line names why.
$poolLoopBaseline = New-Object 'System.Collections.Generic.HashSet[string]'
$poolLoopBaselinePath = Join-Path $Root '.instructions/sanity/baselines/perf-pool-loops.txt'
if (Test-Path -LiteralPath $poolLoopBaselinePath) {
  foreach ($l in (Get-SanityLines $poolLoopBaselinePath)) {
    $t = $l.Trim()
    if ($t -eq '' -or $t.StartsWith('#')) { continue }
    [void]$poolLoopBaseline.Add((($t -split '\|')[0]).Trim().Replace('\', '/'))
  }
}
foreach ($f in $goFiles) {
  $lines = Get-SanityLines $f.Full
  for ($i = 0; $i -lt $lines.Length; $i++) {
    if ($lines[$i] -notmatch '^\s*for\b.*\brange\b|rows\.Next\(\)') { continue }
    # A loop whose per-item query is deliberate says so in a comment naming
    # what bounds it ("bounded by", "bounded because", ...).
    $preamble = ''
    for ($k = [Math]::Max(0, $i - 4); $k -le $i; $k++) { $preamble += "`n" + $lines[$k] }
    if ($preamble -match '(?i)\bbounded\b') { continue }
    $indent = ([regex]::Match($lines[$i], '^\s*')).Length
    $limit = [Math]::Min($i + 60, $lines.Length)
    for ($j = $i + 1; $j -lt $limit; $j++) {
      $ind = ([regex]::Match($lines[$j], '^\s*')).Length
      if ($lines[$j].Trim() -eq '}' -and $ind -le $indent) { break }
      if ($lines[$j] -notmatch $script:SanityDbCallPattern -or $lines[$j] -match $script:SanityDbCallNotMatch) { continue }
      if ($lines[$j] -match '\b\w*[Ss]tmt\.(Query|QueryRow|Exec)(Context)?\(') { break }
      $hit = New-SanityHit -File $f.Rel -Line ($j + 1) -Text ("inside loop starting line {0}: {1}" -f ($i + 1), $lines[$j].Trim())
      if ($lines[$j] -match '\.(Query|QueryRow)(Context)?\(') { $nplus1Read += $hit }
      elseif ($lines[$j] -match '\b\w*[Tt]x\w*\.(Exec)(Context)?\(') { $nplus1Write += $hit }
      elseif (-not $poolLoopBaseline.Contains($f.Rel.Replace('\', '/'))) { $nplus1PoolWrite += $hit }
      break
    }
  }
}
Add-SanityCheck -Report $report -Id '1a' -Title 'Read executed once per item inside a loop (N+1)' -Hits $nplus1Read -Mode review -Severity high `
  -Remediation 'Batch with IN (...) placeholders or a JOIN. If the loop count is genuinely bounded, say what bounds it in a comment above the loop.' `
  -Command 'rg -n "for |range |rows\.Next\(" src/backend/modules src/backend/core'
Add-SanityCheck -Report $report -Id '1b' -Title 'Write executed once per item inside a transaction loop (inventory)' -Hits $nplus1Write -Mode info -Severity low `
  -Remediation 'Expected inside a transaction; prepare the statement once outside the loop where the loop is hot.'
Add-SanityCheck -Report $report -Id '1c' -Title 'Write on the pool (not a transaction) once per item inside a loop, unmarked' -Hits $nplus1PoolWrite -Mode review -Severity medium `
  -Remediation 'Wrap the loop in a transaction when partial failure is a consistency problem or the loop is hot; otherwise add a comment above the loop: // bounded by <what limits the iteration count>.'

# 2. Missing DB indexes on foreign keys
$migText = ''
foreach ($m in (Get-SanityFiles -Root $Root -Paths @('src/backend/core/db/migrations') -Include @('*.sql'))) { $migText += "`n-- FILE " + $m.Rel + "`n" + (Get-SanityText $m.Full) }
$indexed = New-Object 'System.Collections.Generic.HashSet[string]'
foreach ($m in [regex]::Matches($migText, '(?i)CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF NOT EXISTS\s+)?\w+\s+ON\s+(\w+)\s*\(\s*(\w+)')) { $indexed.Add(($m.Groups[1].Value + '.' + $m.Groups[2].Value).ToLowerInvariant()) | Out-Null }
# A table the migration set later renames away (the CREATE x_new -> copy ->
# RENAME TO x rebuild pattern) or drops outright never reaches a running
# database, so an index on it would be dead weight — and, for a dropped
# table, a CREATE INDEX that fails outright.
$transient = New-Object 'System.Collections.Generic.HashSet[string]'
foreach ($m in [regex]::Matches($migText, '(?i)ALTER\s+TABLE\s+(\w+)\s+RENAME\s+TO\s+\w+')) { $transient.Add($m.Groups[1].Value.ToLowerInvariant()) | Out-Null }
foreach ($m in [regex]::Matches($migText, '(?i)DROP\s+TABLE\s+(?:IF EXISTS\s+)?(\w+)')) { $transient.Add($m.Groups[1].Value.ToLowerInvariant()) | Out-Null }

$fkHits = @()
foreach ($t in [regex]::Matches($migText, '(?is)CREATE\s+TABLE\s+(?:IF NOT EXISTS\s+)?(\w+)\s*\((.*?)\)\s*;')) {
  $table = $t.Groups[1].Value; $body = $t.Groups[2].Value
  if ($transient.Contains($table.ToLowerInvariant())) { continue }
  foreach ($pk in [regex]::Matches($body, '(?i)PRIMARY\s+KEY\s*\(\s*(\w+)')) { $indexed.Add(($table + '.' + $pk.Groups[1].Value).ToLowerInvariant()) | Out-Null }
  foreach ($uq in [regex]::Matches($body, '(?i)UNIQUE\s*\(\s*(\w+)')) { $indexed.Add(($table + '.' + $uq.Groups[1].Value).ToLowerInvariant()) | Out-Null }
  foreach ($col in [regex]::Matches($body, '(?im)^\s*(\w+)\s+\w+[^,\n]*\b(PRIMARY\s+KEY|UNIQUE)\b')) { $indexed.Add(($table + '.' + $col.Groups[1].Value).ToLowerInvariant()) | Out-Null }
  $fks = @()
  # Column-level REFERENCES only; table-level FOREIGN KEY (...) / CONSTRAINT lines are handled below.
  foreach ($c in [regex]::Matches($body, '(?im)^\s*(?!FOREIGN\b|CONSTRAINT\b|PRIMARY\b|UNIQUE\b|CHECK\b)(\w+)\s+\w+[^,\n]*\bREFERENCES\s+(\w+)')) { $fks += $c.Groups[1].Value }
  foreach ($c in [regex]::Matches($body, '(?i)FOREIGN\s+KEY\s*\(\s*(\w+)\s*\)\s*REFERENCES')) { $fks += $c.Groups[1].Value }
  foreach ($fk in ($fks | Sort-Object -Unique)) {
    if (-not $indexed.Contains(($table + '.' + $fk).ToLowerInvariant())) { $fkHits += "table ``$table`` column ``$fk`` references another table but has no index" }
  }
}
Add-SanityCheck -Report $report -Id 2 -Title 'Foreign-key columns without a supporting index' -Hits $fkHits -Mode review -Severity medium `
  -Remediation 'Add CREATE INDEX idx_<table>_<col> ON <table>(<col>) in a new migration. Tables the migration set later renames away or drops are skipped.' `
  -Command 'rg -n "CREATE INDEX|FOREIGN KEY|REFERENCES " src/backend/core/db/migrations'

# 3. Docker SDK calls without context.WithTimeout
# The receiver is part of the pattern on purpose. Matching the method name
# alone made `Logger.Info(`, `coord.NodeID()` and `svc.Configured()` read as
# Docker SDK calls and buried the five real ones under 37 false positives.
$dockerRecv = '\b(cli|dockerCli|dockerClient|\w*Conn|\w*Client)\.'
$dockerCall = $dockerRecv + '(Container(List|Inspect|Start|Stop|Restart|Remove|Stats|StatsOneShot|Logs|Create|Top|Pause|Unpause|Kill|Rename|Update|ExecCreate|ExecAttach|ExecStart|ExecInspect|Attach|Wait|Export|Diff|CopyToContainer|CopyFromContainer|StatPath|Commit)|Image(List|Pull|Push|Remove|Inspect|InspectWithRaw|Build|Tag|Save|Load|History|Search)|Network(List|Inspect|Create|Remove|Connect|Disconnect)|Volume(List|Create|Remove|Inspect)|(Containers|Images|Volumes|Networks|BuildCache)Prune|Events|Info|Ping|ServerVersion|DiskUsage|Swarm\w+|Node\w+|Service\w+|Task\w+|Secret\w+|Config\w+)\('
$dockerNoTimeout = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend/modules') -Include @('*.go') -ExcludeMatch @('_test\.go$'))) {
  $text = Get-SanityText $f.Full
  $calls = [regex]::Matches($text, $dockerCall).Count
  if ($calls -eq 0) { continue }
  # A stream whose lifetime is the caller's cannot take a deadline; the file
  # says so in a `// No context.WithTimeout` comment naming the reason.
  if ($text -match 'No context\.WithTimeout') { continue }
  if ($text -notmatch 'context\.WithTimeout\(|context\.WithDeadline\(|WithTimeout\(') { $dockerNoTimeout += New-SanityHit -File $f.Rel -Line 0 -Text "$calls Docker SDK call(s), no context.WithTimeout in file" }
}
Add-SanityCheck -Report $report -Id 3 -Title 'Files with Docker SDK calls but no context.WithTimeout' -Hits $dockerNoTimeout -Mode review -Severity high `
  -Remediation 'Wrap every SDK call: ctx, cancel := context.WithTimeout(ctx, 30*time.Second); defer cancel(). Files may receive an already-bounded ctx from a caller; confirm before dismissing.' `
  -Command 'rg -nP "\b(cli|dockerCli|dockerClient|\w*Conn|\w*Client)\.(Container|Image|Network|Volume)\w+\(" src/backend/modules'

# 4. Kubernetes client-go calls without timeouts
$k8sNoTimeout = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend/modules', 'src/backend/core/kubernetes') -Include @('*.go') -ExcludeMatch @('_test\.go$'))) {
  $text = Get-SanityText $f.Full
  $calls = [regex]::Matches($text, '\.(CoreV1|AppsV1|BatchV1|NetworkingV1|RbacV1|StorageV1|Discovery|MetricsV1beta1)\(\)').Count
  if ($calls -eq 0) { continue }
  if ($text -notmatch 'context\.WithTimeout\(|context\.WithDeadline\(') { $k8sNoTimeout += New-SanityHit -File $f.Rel -Line 0 -Text "$calls client-go call(s), no context.WithTimeout in file" }
}
Add-SanityCheck -Report $report -Id 4 -Title 'Files with Kubernetes client-go calls but no context.WithTimeout' -Hits $k8sNoTimeout -Mode review -Severity high `
  -Remediation 'Use context.WithTimeout for every clientset call; an unreachable API server otherwise hangs the request.' `
  -Command 'rg -n "clientset|Pods\(|Deployments\(|Services\(|Namespaces\(" src/backend/modules src/backend/core/kubernetes'

# 5. Unbounded SELECT queries
#
# Three sources of noise were removed here after the whole 100-hit list was
# read by hand:
#
#   * The block scanner stops at the first backtick, but a query with a
#     dynamic WHERE is written as `SELECT ...` + where + `... LIMIT ? OFFSET ?`,
#     so the LIMIT lived past the block. The statement window below is what the
#     LIMIT test reads.
#   * A SELECT with no FROM is a scalar (`SELECT version()`), a column-shape
#     probe (`WHERE 1 = 0`), or a string in an error message.
#   * core/db is the migration and introspection engine. It reads whole tables
#     and whole catalogs by design; a cap there would corrupt a migration.
$unbounded = @()
foreach ($f in $goFiles) {
  if ($f.Rel -like 'src/backend/core/db/*') { continue }
  # The simulated estate emits sample log text ("SELECT * FROM orders ...") and
  # never opens a database, so a SELECT there is prose, not a query.
  if ($f.Rel -like 'src/backend/core/demo/estate/*') { continue }
  $lines = Get-SanityLines $f.Full
  for ($i = 0; $i -lt $lines.Length; $i++) {
    if ($lines[$i] -cnotmatch '\bSELECT\s' -or $lines[$i] -match '^\s*//') { continue }
    $block = $lines[$i]
    $limit = [Math]::Min($i + 15, $lines.Length)
    $closed = $lines[$i] -match '`\s*[,)]|"\s*[,)]'
    for ($j = $i + 1; -not $closed -and $j -lt $limit; $j++) {
      $block += "`n" + $lines[$j]
      if ($lines[$j] -match '`|"\s*[,)+]') { $closed = $true }
    }
    $window = ($lines[$i..([Math]::Min($i + 24, $lines.Length - 1))] -join "`n")
    if ($window -match '(?i)\bLIMIT\b|\bTOP\s+\d+\b|COUNT\(|sqlite_master|PRAGMA|EXISTS\s*\(') { continue }
    if ($block -match '(?i)\bWHERE\b[^`"]*\b(id|\w+_id|slug|name|key|username|email|token_hash|hash|session_id|user_id|environment_id)\s*=\s*\?') { continue }
    if ($block -match '(?i)\bSELECT\s+\d+\b|\bSELECT\s+1\b|MAX\(|MIN\(|SUM\(|AVG\(') { continue }
    if ($block -notmatch '(?i)\bFROM\b') { continue }
    if ($block -match '(?i)\bWHERE\s+1\s*=\s*0\b') { continue }
    if ($block -match '(?i)\b(information_schema|pg_tables|pg_database|pg_catalog|pg_index|pg_class|pg_namespace)\b') { continue }
    # A named SQL fragment is not a statement. Its call sites append the WHERE
    # and the LIMIT and are checked on their own lines.
    if ($lines[$i] -match '^\s*const\s+\w+\s*=\s*`SELECT') { continue }
    if ($i -gt 0 -and $lines[$i - 1] -match '^\s*const\s+\w+\s*=\s*`\s*$') { continue }
    # A UNION arm belongs to the statement that opens with the first SELECT,
    # and a parenthesised subquery to the statement that contains it.
    if ($i -gt 0 -and $lines[$i - 1] -match '^\s*UNION(\s+ALL)?\s*$') { continue }
    if ($lines[$i] -match '^\s*\(\s*SELECT\b') { continue }
    # IN with generated placeholders is bounded by the argument list; IN with a
    # subquery is one statement, checked on the outer statement's own line.
    if ($block -match '(?i)\bIN\s*\((%s|\?|SELECT\b)') { continue }
    # ... including when the placeholder list is spliced in from Go.
    if ($block -match '(?i)\bIN\s*\([`"]\s*\+') { continue }
    # An equality on any column bounds the result set as surely as one on the
    # hand-listed key columns: `WHERE origin = ?`, `WHERE category = 'audit'`.
    if ($block -match '(?i)\bWHERE\b[\s\S]*?\b\w+\s*=\s*(\?|\$\d+|''[^'']*''|DB_NAME\(\)|current_schema\(\))') { continue }
    # ... including a reserved-word column quoted through QuoteIdent: `WHERE "+QuoteIdent(d, "key")+" = ?"`.
    if ($block -match 'QuoteIdent\([^)]*\)\s*\+\s*"\s*=\s*\?') { continue }
    # A whole-table sweep that must not be truncated says so in a comment. The
    # comment sits above the Query call, which is one or more lines above the
    # SELECT itself, so the lookback covers the whole call preamble.
    $preamble = ''
    for ($k = [Math]::Max(0, $i - 4); $k -lt $i; $k++) { $preamble += "`n" + $lines[$k] }
    if ($block -match 'No LIMIT:|bounded by' -or $preamble -match 'No LIMIT:|bounded by') { continue }
    # A query built inside a helper function is bounded when every call site of
    # that helper hands it a clause carrying a LIMIT.
    $fnName = $null
    for ($k = $i; $k -ge [Math]::Max(0, $i - 40); $k--) {
      $fm = [regex]::Match($lines[$k], '^func\s+(?:\([^)]*\)\s*)?(\w+)\s*\(')
      if ($fm.Success) { $fnName = $fm.Groups[1].Value; break }
    }
    if ($fnName) {
      $useRx = '\b' + [regex]::Escape($fnName) + '\('
      $uses = @()
      for ($u = 0; $u -lt $lines.Length; $u++) {
        if ($lines[$u] -notmatch $useRx -or $lines[$u] -match '^\s*//' -or $lines[$u] -match '^func\s') { continue }
        $uses += (($lines[$u..([Math]::Min($u + 3, $lines.Length - 1))]) -join "`n")
      }
      if ($uses.Count -gt 0 -and @($uses | Where-Object { $_ -notmatch '(?i)\bLIMIT\b' }).Count -eq 0) { continue }
    }
    $unbounded += New-SanityHit -File $f.Rel -Line ($i + 1) -Text $lines[$i].Trim()
  }
}
Add-SanityCheck -Report $report -Id 5 -Title 'SELECT statements with no LIMIT and no key lookup' -Hits $unbounded -Mode review -Severity medium `
  -Remediation 'Paginate with response.ParsePagination or add LIMIT 1000 as a safety cap on admin-wide scans.' `
  -Command 'rg -n "SELECT " src/backend/modules src/backend/core'

# 6. Frontend rerender traps
$inlineStyle = @(Find-Pattern -Root $Root -Paths @('src/frontend') -Include @('*.tsx') -Pattern 'style=\{\{')
$columnsInRender = @(Find-Pattern -Root $Root -Paths @('src/frontend/modules', 'src/frontend/resources') -Include @('*.tsx') -ExcludeMatch @('\.test\.tsx$', '\.stories\.tsx$') -Pattern '^\s{2,}const columns(\s*:[^=]+)?=\s*\[' -NotMatch 'useMemo')
Add-SanityCheck -Report $report -Id '6a' -Title 'Inline style objects in TSX' -Hits $inlineStyle -Mode strict -Severity medium `
  -Remediation 'Replace with Tailwind utilities or arbitrary values (min-h-[300px]).' -Command 'rg -n "style=\{\{" src/frontend'
Add-SanityCheck -Report $report -Id '6b' -Title 'Column definitions rebuilt inside render' -Hits $columnsInRender -Mode review -Severity low `
  -Remediation 'Hoist static column arrays to module scope or wrap in useMemo.' -Command 'rg -n "const columns = \[" src/frontend/modules src/frontend/resources'

# 7. Polling discipline
$noPolling = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/frontend/modules', 'src/frontend/resources') -Include @('*.ts', '*.tsx') -ExcludeMatch @('\.test\.', '\.stories\.'))) {
  # "log" has to be the word, not the tail of Catalog or Dialog — matching the
  # substring put every catalog hook and every dialog in a polling check.
  if ($f.Rel -notmatch '(?i)(dashboard|(?<!cata)(?<!dia)logs?|metrics|activity|events|health|alerts|monitor|stats|status|live)') { continue }
  $text = Get-SanityText $f.Full
  if ($text -notmatch 'useQuery[<(]|useQueries[<(]') { continue }
  # A screen that refreshes on demand rather than on a timer says why.
  if ($text -match 'no polling:') { continue }
  if ($text -notmatch 'refetchInterval|EventSource|WebSocket|useSSE|useEventStream|useWebSocket') { $noPolling += New-SanityHit -File $f.Rel -Line 0 -Text 'useQuery without refetchInterval or a live stream' }
}
Add-SanityCheck -Report $report -Id 7 -Title 'Live-data hooks without refetchInterval or a stream' -Hits $noPolling -Mode review -Severity low `
  -Remediation 'Dashboard, logs, metrics and list hooks should poll (refetchInterval) or subscribe; detail/mutation queries are accepted one-shot.' `
  -Command 'rg -n "useQuery\(|refetchInterval" src/frontend/modules src/frontend/resources'

# 8. Large bundle imports
$bigImports = @(Find-Pattern -Root $Root -Paths @('src/frontend') -Include @('*.ts', '*.tsx') -Pattern "from ['`"](lodash|date-fns|react-icons|moment|rxjs)['`"]")
Add-SanityCheck -Report $report -Id 8 -Title 'Whole-library imports (lodash, date-fns, react-icons)' -Hits $bigImports -Mode strict -Severity medium `
  -Remediation 'Import the specific submodule (lodash/debounce, date-fns/format) or use @tabler/icons-react.' -Command "rg -n `"from 'lodash'|from 'date-fns'|from 'react-icons'`" src/frontend"

# 9. Concurrent stats collection
$seqStats = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend/modules') -Include @('*.go') -ExcludeMatch @('_test\.go$'))) {
  $text = Get-SanityText $f.Full
  if ($text -notmatch '\.(ContainerStats|ContainerStatsOneShot|GetAllContainerStats|AllContainerStats|BulkStats)\(') { continue }
  if ($text -match 'go func\(|sync\.WaitGroup|errgroup|semaphore') { continue }
  # The cost only appears when the call is *inside* a loop: one stats call per
  # container over the agent transport is ~2s. A file that fetches a single
  # named container's stats and then ranges over that one response's networks
  # or block-IO entries is not a bulk fetch, so require a range/for within the
  # 12 lines preceding the call rather than anywhere in the file.
  $lines = Get-SanityLines $f.Full
  for ($i = 0; $i -lt $lines.Count; $i++) {
    if ($lines[$i] -notmatch '\.(ContainerStats|ContainerStatsOneShot|GetAllContainerStats|AllContainerStats|BulkStats)\(') { continue }
    $from = [Math]::Max(0, $i - 12)
    $window = ($lines[$from..$i] -join "`n")
    if ($window -match '(?m)^\s*for\s.*\brange\b') {
      $seqStats += New-SanityHit -File $f.Rel -Line ($i + 1) -Text 'stats fetched inside a range loop with no goroutine/WaitGroup in file'
      break
    }
  }
}
Add-SanityCheck -Report $report -Id 9 -Title 'Bulk container stats fetched sequentially' -Hits $seqStats -Mode review -Severity high `
  -Remediation 'One-shot stats cost ~2s per container over the agent transport. Fan out with goroutines + WaitGroup (bounded).' `
  -Command 'rg -n "GetAllContainerStats|AllContainerStats|ContainerStats" src/backend/modules'

# 10. Agent environment guarding in collectors
$collectorsUnguarded = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend/modules') -Include @('collector.go', 'collector_*.go', 'poller.go', 'scheduler.go') -ExcludeMatch @('_test\.go$'))) {
  $text = Get-SanityText $f.Full
  if ($text -notmatch 'ContainerStats|StatsOneShot|Inspect|DiskUsage|Events\(') { continue }
  if ($text -notmatch '(?i)IsAgentEnv|ConnectionType\s*==\s*"agent"|connection_type|"agent"|agent env|IsAgent\(|isAgent') { $collectorsUnguarded += New-SanityHit -File $f.Rel -Line 0 -Text 'expensive collector with no agent-environment guard' }
}
$agentGuards = @(Find-Pattern -Root $Root -Paths $goPaths -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern 'IsAgentEnv|connection_type.*agent|agent env|IsAgent\(')
Add-SanityCheck -Report $report -Id '10a' -Title 'Background collectors without an agent-environment guard' -Hits $collectorsUnguarded -Mode review -Severity medium `
  -Remediation 'The background metrics collector must skip agent environments; add the guard or confirm the collector is cheap over the agent transport.' `
  -Command 'rg -n "IsAgentEnv|connection_type.*agent|agent env" src/backend/modules src/backend/core'
Add-SanityCheck -Report $report -Id '10b' -Title 'Agent-environment guard sites (inventory)' -Hits $agentGuards -Mode info -Severity info

Add-SanityNote -Report $report -Text 'Accepted: COUNT(*) queries, primary/unique key lookups, one-shot detail queries, main.go lifecycle setup.'
Add-SanityNote -Report $report -Text 'Checks 3 and 4 are file-level heuristics: a file whose ctx is bounded by its caller is a false positive.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
