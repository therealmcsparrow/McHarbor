# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Security sanity check. Mirrors .instructions/sanity/security.md.
.DESCRIPTION
  Grep-first sweep for SQL construction, hardcoded secrets, XSS sinks,
  WebSocket/CORS misconfiguration, socket exposure, raw error leakage,
  encryption error handling, cookie flags, agent token comparison and
  audit coverage. Read-only; writes .results/sanity/<stamp>-security.md.
.EXAMPLE
  .\scripts\sanity\security\check.ps1
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

$report = New-SanityReport -Name 'security' -Title 'Security Sanity' -Scope 'src/backend, src/frontend, src/agent, src/plugins/Plugins/*/backend' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$goPaths = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules', 'src/backend/core'))
$goArgs = @{ Root = $Root; Paths = $goPaths; ExcludeMatch = @('_test\.go$') }
$goInc = @('*.go')
$beFeAgentScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend', 'src/frontend', 'src/agent'))
$feScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend'))
$beScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend'))
$beModFeScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules', 'src/frontend'))
$rawErrScope = (Get-SanityScope -Root $Root -Paths $Paths -Default (@('src/backend/modules', 'src/backend/core') + @(Get-SanityPluginPaths -Root $Root -SubPath 'backend')))
$rawErrArgs = @{ Root = $Root; Paths = $rawErrScope; ExcludeMatch = @('_test\.go$') }
$migrationScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/core/db/migrations'))
$agentTokenScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules/agent', 'src/backend/core', 'src/backend/modules/api_keys', 'src/backend/modules/auth'))

# 1. SQL injection / fragile query construction
#
# The danger is a *value* reaching the SQL text. Two things are not that, and
# flagging them buried the check under its own idiom:
#
#   * An identifier put through QuoteIdent(driver, name) — that helper exists
#     precisely so a table or column name can be spliced safely.
#   * A fragment the code itself wrote: a placeholder list, a slice of
#     `col = ?` conditions, a constant column list. The values still travel as
#     query arguments.
#   * An identifier the code proves cannot come from a request — a typed
#     constant, say — where the line carries a `SQL identifier:` comment
#     naming what constrains it.
$sqlSafeBuild = 'QuoteIdent\(|strings\.Join\(\s*\w*(placeholder|mark|condition|where|set|assignment|quoted|column|clause|part|arg)\w*\b|\+\s*\w*([Cc]olumns|[Qq]uoted)\b|\+\s*placeholders\b|SQL identifier:'
$sqlArgs = @{ Root = $Root; Paths = $goPaths; ExcludeMatch = @('_test\.go$', 'core[\\/]db[\\/]dialect_[^\\/]+\.go$', 'core[\\/]sqldialect[\\/]') }
$sqlFmt = @(Find-Pattern @sqlArgs -Include $goInc -Pattern '(fmt\.Sprintf|strings\.Join)\(' -AlsoMatch '\b(SELECT|INSERT|UPDATE|DELETE|WHERE|ORDER BY|FROM|SET)\b' -NotMatch $sqlSafeBuild)
$sqlConcat = @(Find-Pattern @sqlArgs -Include $goInc -Pattern '\.(Query|QueryRow|Exec|QueryContext|QueryRowContext|ExecContext)\([^)]*"\s*\+' -NotMatch $sqlSafeBuild)
Add-SanityCheck -Report $report -Id 1 -Title 'SQL built with Sprintf/Join or string concatenation' -Hits ($sqlFmt + $sqlConcat) -Mode review -Severity high `
  -Remediation 'Use static query strings with placeholder arguments; whitelist ORDER BY / WHERE column names before formatting.' `
  -Command 'rg -n "fmt\.Sprintf\(|strings\.Join\(|SELECT |INSERT |UPDATE |DELETE " src/backend/modules src/backend/core'

# 2. Secrets in code
$secretAssign = @(Find-Pattern -Root $Root -Paths $beFeAgentScope -Include @('*.go', '*.ts', '*.tsx') -ExcludeMatch @('_test\.go$', '\.test\.tsx?$') `
  -Pattern '(?i)\b(api[_-]?key|secret|password|passwd|token|private[_-]?key)\b\s*[:=]\s*["''][^"''\s]{8,}["'']' `
  -NotMatch '(?i)(placeholder|example|\bt\(|\$\{|^\s*//|^\s*\*|\.env|process\.env|Getenv|i18n|json:"|form:"|yaml:"|type:\s*|Header\(|X-[A-Z]|Bearer |=\s*"\*+"|\*{3,}|[:=]\s*["''][a-z][a-z0-9]*(\.[a-zA-Z][a-zA-Z0-9]*)+["'']|\bSecret:\s*["''][a-z0-9]+(_[a-z0-9]+)+["''])')
$pem = @(Find-Pattern -Root $Root -Paths $beFeAgentScope -Include @('*.go', '*.ts', '*.tsx', '*.json', '*.yml', '*.yaml', '*.env') -ExcludeMatch @('_test\.go$', '\.test\.tsx?$') `
  -Pattern 'BEGIN (OPENSSH|RSA|EC|DSA|PGP) PRIVATE KEY' -NotMatch '([Pp]laceholder"\s*:\s*"-----BEGIN [A-Z ]+-----"\s*,?\s*$|Contains|HasPrefix|regexp|match|placeholder|\bt\(|startsWith|includes\(|^\s*//)')
Add-SanityCheck -Report $report -Id 2 -Title 'Hardcoded credentials or private keys' -Hits ($secretAssign + $pem) -Mode review -Severity high `
  -Remediation 'Move real secrets to environment/config and rotate them. Type names, field names and translation labels are accepted.' `
  -Command 'rg -n -i "(api[_-]?key|secret|password|token|private[_-]?key|BEGIN OPENSSH PRIVATE KEY|BEGIN RSA PRIVATE KEY)" src'

# 3. XSS sinks
$xss = @(Find-Pattern -Root $Root -Paths $feScope -Include @('*.ts', '*.tsx') -Pattern 'dangerouslySetInnerHTML|innerHTML\s*=|outerHTML\s*=|insertAdjacentHTML|document\.write\(')
Add-SanityCheck -Report $report -Id 3 -Title 'XSS sinks (innerHTML, dangerouslySetInnerHTML, document.write)' -Hits $xss -Mode review -Severity high `
  -Remediation 'Confirm every sink receives sanitized HTML (DOMPurify or a trusted static source); prefer React rendering.' `
  -Command 'rg -n "dangerouslySetInnerHTML|innerHTML\s*=|outerHTML\s*=|insertAdjacentHTML|document\.write\(" src/frontend'

# 4. WebSocket origin and auth bypass patterns
$wsOrigin = @(Find-Pattern -Root $Root -Paths $beScope -Include @('*.go') -Multiline -Pattern 'CheckOrigin:\s*func\([^)]*\)\s*bool\s*\{\s*return true\s*\}')
$rawAuthErr = @(Find-Pattern -Root $Root -Paths $beScope -Include @('*.go') -Pattern 'response\.(Unauthorized|BadRequest|InternalError|Forbidden|NotFound|Conflict)\(w,\s*(r,\s*)?err\.Error\(')
Add-SanityCheck -Report $report -Id 4 -Title 'WebSocket CheckOrigin always true / raw auth error leakage' -Hits ($wsOrigin + $rawAuthErr) -Mode strict -Severity high `
  -Remediation 'Validate the Origin header against the configured host; return i18n error codes instead of err.Error().' `
  -Command 'rg -n "CheckOrigin:\s*func\([^)]*\)\s*bool\s*\{\s*return true\s*\}" src/backend'

# 5. CORS misconfiguration
$corsAll = @(Find-Pattern -Root $Root -Paths $beScope -Include @('*.go') -Pattern 'AllowedOrigins' -AlsoMatch '"\*"')
$corsInventory = @(Find-Pattern -Root $Root -Paths $beScope -Include @('*.go') -Pattern 'AllowedOrigins|AllowCredentials|cors\.Handler')
Add-SanityCheck -Report $report -Id '5a' -Title 'CORS wildcard AllowedOrigins' -Hits $corsAll -Mode strict -Severity high `
  -Remediation 'Never pair "*" with AllowCredentials: true; use an explicit origin allow-list.' -Command 'rg -n "AllowedOrigins|AllowCredentials|cors\.Handler" src/backend'
Add-SanityCheck -Report $report -Id '5b' -Title 'CORS configuration inventory' -Hits $corsInventory -Mode info -Severity info -Note 'Inspect AllowCredentials next to the origin list.'

# 6. Docker socket exposure
$sock = @(Find-Pattern -Root $Root -Paths $beModFeScope -Include @('handler.go', '*.ts', '*.tsx') -ExcludeMatch @('\.test\.tsx?$') -Pattern '/var/run/docker\.sock|docker\.sock|npipe:////\./pipe/docker_engine' -NotMatch '(\bt\(|i18n|placeholder|description|label)')
# The finding is a socket path that came *from the server*. A literal the UI
# owns — the install command an operator has to run, the default value of the
# field they type into — is neither a leak nor server config. Such a file says
# so in a `docker socket:` comment naming which of the two it is.
$sock = @($sock | Where-Object { (Get-SanityText (Join-Path $Root $_.File)) -notmatch 'docker socket:' })
Add-SanityCheck -Report $report -Id 6 -Title 'Docker socket path in handlers or frontend' -Hits $sock -Mode review -Severity medium `
  -Remediation 'Socket paths belong in core/docker connection setup only; never in API responses or logs shown to users.' `
  -Command 'rg -n "/var/run/docker\.sock|docker\.sock|npipe:////./pipe/docker_engine" src/backend src/frontend'

# 7. Raw internal error exposure
$rawErrHandlers = @(Find-Pattern @rawErrArgs -Include $goInc -Pattern 'response\.\w+\(w,\s*(r,\s*)?[^)]*err\.Error\(\)')
$rawErrDTO = @(Find-Pattern @rawErrArgs -Pattern '(Error|Message|Detail|Reason)\s*:\s*err\.Error\(\)' -Include @('handler.go', 'model.go', 'service.go'))
$rawErrAll = @(Find-Pattern @rawErrArgs -Include $goInc -Pattern 'err\.Error\(\)')
Add-SanityCheck -Report $report -Id '7a' -Title 'Handlers returning err.Error() to clients' -Hits ($rawErrHandlers + $rawErrDTO) -Mode strict -Severity high `
  -Remediation 'Log the real error server-side (h.app.Logger.Error) and respond with response.*Code(w, r, i18n.ErrX).' -Command 'rg -n "err\.Error\(\)" src/backend/modules src/backend/core src/plugins/Plugins/*/backend'
Add-SanityCheck -Report $report -Id '7b' -Title 'All err.Error() call sites (inventory)' -Hits $rawErrAll -Mode info -Severity info -Note 'Server-side logging and internal wrapping are accepted; only client-facing use is a finding.'

# 8. Encryption of sensitive fields
$encIgnored = @(Find-Pattern @goArgs -Include $goInc -Pattern '(,\s*_\s*:?=|^\s*_\s*=)\s*[\w.]*(Encrypt|Decrypt)\(')
# 8b. Every sensitive-looking migration column has a recorded protection.
#
# Columns are read from CREATE TABLE bodies and ALTER TABLE ... ADD COLUMN, so
# seed INSERTs, CREATE INDEX lines and comments are never mistaken for columns.
# The matrix (encrypted-columns.json) is matched by table + column.
$sensName = '(?i)(agent_token|ssh_key|tls_key|tls_cert|tls_ca|kubeconfig|bearer_token|password|passphrase|secret|api_key|apikey|private_key|credential|(^|_)token($|_)|(^|_)tokens?_(enc|encrypted)$)'
$sensSkip = '(?i)(_hash$|^has_|^is_|^must_|_locked|_until$|_url$|_at$|_count$|_ttl|_expires|_seconds$|_id$|^max_|^used_|_used$|_len(gth)?$|_type$|_kind$|_prefix$|_label$|_name$|_hint$|_status$|_mode$|_enabled$|_required$|_set$)'
$colType = '(?i)^(TEXT|INTEGER|INT|BIGINT|BLOB|BOOLEAN|BOOL|REAL|NUMERIC|VARCHAR|CHAR|TIMESTAMP|DATETIME|JSON|JSONB|UUID|BYTEA|DOUBLE|FLOAT)\b'
$matrixPath = Join-Path $PSScriptRoot 'encrypted-columns.json'
$matrix = @{}
$matrixRows = @()
if (Test-Path -LiteralPath $matrixPath) { try { $matrixRows = @(ConvertFrom-SanityJson (Get-SanityText $matrixPath)) } catch { $matrixRows = @() } }
foreach ($mr in $matrixRows) { $matrix[(("$(Get-SanityJsonValue $mr 'table')." + "$(Get-SanityJsonValue $mr 'column')").ToLowerInvariant())] = "$(Get-SanityJsonValue $mr 'protection')" }
$migFiles = @(Get-SanityFiles -Root $Root -Paths $migrationScope -Include @('*.sql')) + @(Get-SanityFiles -Root $Root -Paths @(Get-SanityPluginPaths -Root $Root -SubPath 'migrations') -Include @('*.sql'))
$colSeen = New-Object 'System.Collections.Generic.HashSet[string]'
$colPlain = @(); $colUnknown = @(); $colCounts = @{}
foreach ($mf in $migFiles) {
  $mtext = Get-SanityText $mf.Full
  if ([string]::IsNullOrEmpty($mtext)) { continue }
  $cand = @()
  foreach ($t in [regex]::Matches($mtext, '(?is)CREATE\s+TABLE\s+(?:IF NOT EXISTS\s+)?[''"`]?(\w+)[''"`]?\s*\((.*?)\)\s*;')) {
    $tbl = $t.Groups[1].Value; $body = $t.Groups[2].Value; $bodyStart = $t.Groups[2].Index
    $depth = 0; $start = 0
    for ($ci = 0; $ci -le $body.Length; $ci++) {
      $ch = if ($ci -lt $body.Length) { $body[$ci] } else { ',' }
      if ($ch -eq '(') { $depth++ } elseif ($ch -eq ')') { $depth-- }
      elseif ($ch -eq ',' -and $depth -eq 0) {
        $piece = $body.Substring($start, $ci - $start)
        $clean = ($piece -split "`r?`n" | Where-Object { $_ -notmatch '^\s*--' }) -join ' '
        $dm = [regex]::Match($clean.Trim(), '^[''"`]?(\w+)[''"`]?\s+(\w+)')
        if ($dm.Success -and $dm.Groups[1].Value -notmatch '(?i)^(PRIMARY|FOREIGN|UNIQUE|CONSTRAINT|CHECK|KEY)$' -and $dm.Groups[2].Value -match $colType) {
          $cand += [pscustomobject]@{ Table = $tbl; Column = $dm.Groups[1].Value; Index = $bodyStart + $start }
        }
        $start = $ci + 1
      }
    }
  }
  foreach ($a in [regex]::Matches($mtext, '(?i)ALTER\s+TABLE\s+[''"`]?(\w+)[''"`]?\s+ADD\s+(?:COLUMN\s+)?[''"`]?(\w+)[''"`]?')) {
    $cand += [pscustomobject]@{ Table = $a.Groups[1].Value; Column = $a.Groups[2].Value; Index = $a.Index }
  }
  foreach ($c in $cand) {
    if ($c.Column -notmatch $sensName -or $c.Column -match $sensSkip) { continue }
    $c.Table = $c.Table -replace '(?i)_(new|old|tmp)$', ''
    $key = ($c.Table + '.' + $c.Column).ToLowerInvariant()
    $line = Get-SanityLineNumber -Text $mtext -Index $c.Index
    if ($matrix.ContainsKey($key)) {
      if (-not $colSeen.Add($key)) { continue }
      $prot = $matrix[$key]
      $colCounts[$prot] = 1 + [int]$colCounts[$prot]
      if ($prot -eq 'plaintext') { $colPlain += New-SanityHit -File $mf.Rel -Line $line -Text ("{0}.{1} is recorded as plaintext in encrypted-columns.json" -f $c.Table, $c.Column) }
    } else {
      if (-not $colSeen.Add($key)) { continue }
      $colUnknown += New-SanityHit -File $mf.Rel -Line $line -Text ("{0}.{1} looks sensitive and is not in scripts/sanity/security/encrypted-columns.json - add it with its protection (encrypted|hashed|secret-ref|superseded|plaintext|plaintext-accepted)" -f $c.Table, $c.Column)
    }
  }
}
$encCalls = @(Find-Pattern @goArgs -Include $goInc -Pattern '\.(Encrypt|Decrypt)\(')
Add-SanityCheck -Report $report -Id '8a' -Title 'Ignored encryption/decryption errors' -Hits $encIgnored -Mode strict -Severity high `
  -Remediation 'Always check encryption.Encrypt/Decrypt errors; a swallowed error stores plaintext or returns garbage.' -Command 'rg -n "Encrypt\(|Decrypt\(" src/backend/modules src/backend/core'
Add-SanityCheck -Report $report -Id '8b' -Title 'Sensitive columns recorded as plaintext in the protection matrix' -Hits $colPlain -Mode strict -Severity high `
  -Remediation 'Encrypt the column in its module service (encryption.Encrypt/Decrypt), then set it to encrypted in encrypted-columns.json.'
Add-SanityCheck -Report $report -Id '8c' -Title 'Sensitive-looking migration columns missing from the protection matrix' -Hits $colUnknown -Mode strict -Severity high `
  -Remediation 'Add the table and column to scripts/sanity/security/encrypted-columns.json with its protection and the write-path evidence.'
Add-SanityCheck -Report $report -Id '8d' -Title 'Protection matrix inventory' -Hits @($colCounts.Keys | Sort-Object | ForEach-Object { "{0}: {1} column(s) present in migrations" -f $_, $colCounts[$_] }) -Mode info -Severity info `
  -Note ("{0} matrix rows, {1} Encrypt/Decrypt call sites in Go code." -f $matrixRows.Count, $encCalls.Count)

# 9. Session cookie security
$tlsOnly = @(Find-Pattern @goArgs -Include $goInc -Pattern 'Secure:\s*r\.TLS\s*!=\s*nil')
$cookieLiterals = @(Find-Pattern -Root $Root -Paths $goPaths -Include @('*.go') -ExcludeMatch @('_test\.go$') -Multiline -Pattern '&?http\.Cookie\{[^}]*\}')
$noHttpOnly = @($cookieLiterals | Where-Object { $_.Match -notmatch 'HttpOnly:\s*true' -and $_.Match -notmatch 'MaxAge:\s*-1' -and $_.Match -notmatch 'Expires:\s*time\.Unix\(0' })
$noSameSite = @($cookieLiterals | Where-Object { $_.Match -notmatch 'SameSite:' -and $_.Match -notmatch 'MaxAge:\s*-1' })
$secureHelpers = @(Find-Pattern -Root $Root -Paths $beScope -Include @('*.go') -Pattern 'FORCE_SECURE_COOKIES|X-Forwarded-Proto|ShouldSetSecureCookie')
Add-SanityCheck -Report $report -Id '9a' -Title 'Cookies relying only on r.TLS for Secure flag' -Hits $tlsOnly -Mode strict -Severity medium `
  -Remediation 'Use the shared ShouldSetSecureCookie helper (honours FORCE_SECURE_COOKIES and X-Forwarded-Proto) behind reverse proxies.' -Command 'rg -n "http\.SetCookie|Secure:\s|SameSite:|HttpOnly:" src/backend'
Add-SanityCheck -Report $report -Id '9b' -Title 'http.Cookie literals without HttpOnly: true' -Hits $noHttpOnly -Mode review -Severity medium -Remediation 'Auth and session cookies must set HttpOnly: true. Deletion cookies (MaxAge -1) are excluded.'
Add-SanityCheck -Report $report -Id '9c' -Title 'http.Cookie literals without SameSite' -Hits $noSameSite -Mode review -Severity low -Remediation 'Set SameSite (Lax or Strict) on every non-deletion cookie.'
Add-SanityCheck -Report $report -Id '9d' -Title 'Secure-cookie helper usage (inventory)' -Hits $secureHelpers -Mode info -Severity info

# 10. Agent token handling
$tokenEq = @(Find-Pattern -Root $Root -Paths $agentTokenScope -Include @('*.go') -ExcludeMatch @('_test\.go$') `
  -Pattern '[!=]=\s*(token|req\.Token|expectedToken|storedToken|agentToken|apiKey|secret)\b' -NotMatch '(==\s*""|!=\s*""|len\(|nil\b|\.Type\b|Kind\b|TokenType|tokenType)')
$ctc = @(Find-Pattern -Root $Root -Paths $beScope -Include @('*.go') -Pattern 'ConstantTimeCompare|ValidateAgentToken')
Add-SanityCheck -Report $report -Id '10a' -Title 'Direct equality on token/secret values' -Hits $tokenEq -Mode review -Severity high `
  -Remediation 'Compare hashes with crypto/subtle.ConstantTimeCompare; store only hashes of agent tokens and API keys.' -Command 'rg -n "ConstantTimeCompare|==\s*token|==\s*req\.Token|ValidateAgentToken" src/backend/modules/agent src/backend/core'
$ctcHits = @(); if ($ctc.Count -eq 0) { $ctcHits = @('No ConstantTimeCompare / ValidateAgentToken usage found in src/backend') }
Add-SanityCheck -Report $report -Id '10b' -Title 'Constant-time comparison present' -Hits $ctcHits -Mode review -Severity high -Note ("{0} constant-time comparison sites found." -f $ctc.Count)

# 11. Audit coverage for secret/token reveal paths
$auditScript = Join-Path $Root 'scripts\old\audit_coverage_check.ps1'
if (-not (Test-Path -LiteralPath $auditScript)) { $auditScript = Join-Path $Root 'scripts\audit_coverage_check.ps1' }
if (Test-Path -LiteralPath $auditScript) {
  $auditRaw = ''
  try {
    $auditRaw = (@(& $auditScript -Root $Root 2>&1 | ForEach-Object { "$_" }) -join "`n")
  } catch {
    $auditRaw = "audit_coverage_check.ps1 failed: $($_.Exception.Message)"
  }
  $auditLines = @($auditRaw -split "`r?`n")
  $violationCount = -1
  foreach ($l in $auditLines) { if ($l -match '^Violations:\s*(\d+)') { $violationCount = [int]$Matches[1] } }
  $coverageLine = (@($auditLines | Where-Object { $_ -match '^(Coverage|Required host routes|Covered host routes):' }) -join '; ')
  $inViolations = $false
  $violations = @()
  foreach ($l in $auditLines) {
    if ($l -match '^## Violations') { $inViolations = $true; continue }
    if ($inViolations -and $l -match '^- ') { $violations += $l.Substring(2) }
  }
  if ($violationCount -lt 0) { $violations = @('audit_coverage_check.ps1 produced no "Violations:" line; output: ' + (($auditLines | Select-Object -Last 3) -join ' / ')) }
  Add-SanityCheck -Report $report -Id '11a' -Title 'Audit coverage of mutating and secret-revealing routes' -Hits $violations -Mode strict -Severity high `
    -Remediation 'Add AuditLog.Log / LogWithUser / Audit.Emit, or an explicit // AuditExempt: rationale; plugin routes need audit_action + audit_entity_type.' `
    -Note ("Delegated to {0}. {1}. That script also refreshes .results/audit-coverage/ and .references/audit/action-route-map.json." -f (Get-SanityRelPath -Root $Root -Full $auditScript), $coverageLine) `
    -Command 'powershell -NoProfile -ExecutionPolicy Bypass -File scripts/old/audit_coverage_check.ps1'
} else {
  Add-SanityCheck -Report $report -Id '11a' -Title 'Audit coverage of mutating and secret-revealing routes' -Skipped -SkipReason 'scripts/old/audit_coverage_check.ps1 not found'
}
$reveal = @(Find-Pattern @goArgs -Pattern '(?i)reveal|shown once|install token|regenerate-token|api[_-]?key|secret metadata|credential metadata' -Include @('routes.go', 'handler.go'))
Add-SanityCheck -Report $report -Id '11b' -Title 'Secret/token reveal routes (inventory)' -Hits $reveal -Mode info -Severity info -Note 'Cross-check each against the audit coverage violations above.'

Add-SanityNote -Report $report -Text 'Accepted: type/field names, env-var names, translation labels, UI copy naming connection types, internal connection setup code.'
Add-SanityNote -Report $report -Text 'Mode strict = fail on any hit; review = confirm by reading the file; info = inventory only.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
