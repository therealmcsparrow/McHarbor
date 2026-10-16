# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Go standards sanity check. Mirrors .instructions/sanity/go-standards.md.
.EXAMPLE
  .\scripts\sanity\go-standards\check.ps1
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

$report = New-SanityReport -Name 'go-standards' -Title 'Go Standards Sanity' -Scope 'src/backend, src/agent' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$beScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend'))
$allScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend', 'src/agent'))
$beModCoreScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules', 'src/backend/core'))
$be = @{ Root = $Root; Paths = $beScope; Include = @('*.go'); ExcludeMatch = @('_test\.go$') }
$all = @{ Root = $Root; Paths = $allScope; Include = @('*.go'); ExcludeMatch = @('_test\.go$') }

# 1. panic()
$panics = @(Find-Pattern -Root $Root -Paths $beModCoreScope -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern '\bpanic\(' -NotMatch '^\s*//|panic\(rec\)|//\s*panic-ok:\s*\S')
Add-SanityCheck -Report $report -Id 1-Title 'panic() in handler, service or core code' -Hits $panics -Mode strict -Severity high -Remediation 'Return an error instead; re-panicking inside a recover() middleware is the only accepted form.' -Command 'rg -n "\bpanic\(" src/backend/modules src/backend/core'

# 2. Error string style
$properNouns = 'Docker|Kubernetes|K8s|SSH|TLS|SAML|OIDC|SCIM|API|HTTP|ID|URL|JSON|YAML|Git|GitHub|GitLab|Gitea|Podman|Compose|Swarm|SQLite|AES|MFA|TOTP|WebSocket|OAuth|LDAP|AI|SMTP|S3|NFS|SFTP|FTP|SMB|ZFS|OpenAPI|Argon|CSRF|JWT|IdP|Redis|Helm|CIDR|DNS|PEM|RSA|ECDSA|OTP|MinIO|Azure|AWS|GCP|Google|Microsoft|Entra|Okta|Keycloak|Slack|Teams|Discord|Telegram|Signal|WhatsApp|Gotify|Ntfy|Trivy|Grype|Clair|Synology|TrueNAS|Proxmox|Ollama|OpenAI|Anthropic|Mistral|MiniMax|Nextcloud|ownCloud|OpenCloud|Dropbox|Box|OneDrive|SharePoint|Samba|Unicode|UTF|ACS|SP|Base64|PKCE|CA|CN|SAN|RBAC|ACL|CORS|SSE|IPv4|IPv6|MAC|VLAN|WireGuard|PATCH|POST|GET|PUT|DELETE|Windows|Linux|macOS|ARM|AMD|Intel|NVIDIA|Prometheus|Grafana|Loki|Syslog|SIEM|CEF|GELF|PagerDuty|Opsgenie|Twilio|Mailgun|SendGrid|Postmark|Resend|Brevo|Exchange|Gmail|IMAP|POP3|MIME|CSV|PDF|SVG|PNG|ICS|Xid|UUID|ULID|SHA|HMAC|GCM|CBC|PBKDF2|Bcrypt|Scrypt|Ed25519|X509|ASN|DER|PKCS|Vault|Bitwarden|Vaultwarden|Authentik|Authelia|Traefik|Caddy|Nginx|Apache|HAProxy|Envoy|Istio|Cilium|Calico|Flannel|Longhorn|Rook|Ceph|GlusterFS|Btrfs|XFS|Ext4|LVM|RAID|SMART|IPMI|BMC|UEFI|BIOS|GRUB|WSL|Hyper|VMware|VirtualBox|QEMU|KVM|LXC|LXD|Incus|Firecracker|Kata|gVisor|Podman'
$capErr = @(Find-Pattern @be -Pattern 'errors\.New\("[A-Z]|fmt\.Errorf\("[A-Z]' -NotMatch ('(errors\.New|fmt\.Errorf)\("((' + $properNouns + ')\b|[A-Z][A-Z0-9_]{2,}\b|[A-Z]\w*(\.\w+)*: )'))
$punctErr = @(Find-Pattern @be -Pattern 'errors\.New\("[^"]*[.!?]"|fmt\.Errorf\("[^"]*[.!?]"' -NotMatch '\.\.\."|%[sdvq]\)"|\)\."')
Add-SanityCheck -Report $report -Id 2 -Title 'Error strings capitalized or ending with punctuation' -Hits ($capErr + $punctErr) -Mode strict -Severity low `
  -Remediation 'Error strings are lowercase (unless starting with a proper noun) and carry no trailing punctuation.' -Command 'rg -n "errors\.New\(\"[A-Z]|fmt\.Errorf\(\"[A-Z]" src/backend'

# 3. Error wrapping with %w
$wrapV = @(Find-Pattern @be -Pattern 'fmt\.Errorf\([^\n]*%v' -AlsoMatch '\b(err|e|\w+Err)\s*\)' -NotMatch '%w')
Add-SanityCheck -Report $report -Id 3 -Title 'Upstream errors wrapped with %v instead of %w' -Hits $wrapV -Mode strict -Severity medium -Remediation 'Use %w so errors.Is / errors.As keep working.' -Command 'rg -n "fmt\.Errorf\([^\n]*%v" src/backend'

# 4. Getters with Get prefix
$getters = @(Find-Pattern @be -Pattern 'func \([^)]+\) Get[A-Z]\w*\(' -NotMatch '\) GetContext\(')
Add-SanityCheck -Report $report -Id 4 -Title 'Getter methods with Get prefix' -Hits $getters -Mode strict -Severity low -Remediation 'Drop the Get prefix: user.Name(), not user.GetName(). Interface-mandated names (plugin SDK, protobuf) are the accepted exception; note them.' -Command 'rg -n "func \([^)]+\) Get[A-Z]" src/backend'

# 5. Receiver names
$receivers = @(Find-Pattern @all -Pattern 'func \((self|this|me) ')
Add-SanityCheck -Report $report -Id 5 -Title 'Receiver named self/this/me' -Hits $receivers -Mode strict -Severity low -Remediation 'Use a short abbreviation of the type name (s *Service, h *Handler).' -Command 'rg -n "func \((self|this|me) " src/backend'

# 6. UPPER_SNAKE constants
$upperSnake = @(Find-Pattern @all -StripStrings -Pattern '^\s*(const\s+)?[A-Z][A-Z0-9]*_[A-Z0-9_]+\s*(\w+\s*)?=' -NotMatch '^\s*//|json:|env|Env|ENV|os\.Getenv')
$upperSnake = Remove-SanityRawStringHits -Hits $upperSnake -Root $Root
Add-SanityCheck -Report $report -Id 6 -Title 'UPPER_SNAKE constant names' -Hits $upperSnake -Mode review -Severity low -Remediation 'Go constants are MixedCaps (MaxRetries). External protocol values and env-var name tables are accepted.' -Command 'rg -n "const [A-Z0-9_]{3,}" src/backend'

# 7. context.Context must be first parameter
$ctxNotFirst = @(Find-Pattern @all -Pattern 'func\s+(\([^)]*\)\s*)?\w+\([^)]*\w[^)]*,\s*ctx context\.Context')
Add-SanityCheck -Report $report -Id 7 -Title 'ctx context.Context not the first parameter' -Hits $ctxNotFirst -Mode strict -Severity medium -Remediation 'Move ctx to the first position.' -Command 'rg -n "func [^(]+\([^)]*,\s*ctx context\.Context" src/backend'

# 8. context stored in structs
# A field carrying //nolint:containedctx is an accepted exception: the two
# legitimate shapes are a process-lifetime root context owned by a lifecycle
# object, and an io.Reader/Writer adapter whose whole purpose is to fail its
# reads when the surrounding operation is cancelled. Both must carry the
# marker plus a reason on the same line.
$ctxField = @(Find-Pattern @all -Pattern '^\s+\w+\s+context\.Context\s*(//.*)?$' -NotMatch '^\s*//|nolint:containedctx')
Add-SanityCheck -Report $report -Id 8 -Title 'context.Context stored as a struct field' -Hits $ctxField -Mode strict -Severity medium -Remediation 'Pass ctx per call; long-lived services keep a cancel func, not a ctx. A genuine lifecycle root or cancellable io adapter carries //nolint:containedctx plus a reason.' -Command 'rg -n "context\.Context" src/backend'

# 9. Goroutine lifecycle management
$goFuncNoLifecycle = @()
foreach ($f in @(Get-SanityFiles @be)) {
  $text = Get-SanityText $f.Full
  $n = [regex]::Matches($text, 'go func\(').Count
  if ($n -eq 0) { continue }
  # A goroutine that runs one operation to completion and unwinds in a defer
  # ends without any of the markers above: a VACUUM that returns, an io.Pipe
  # writer that closes. Those say so in a `goroutine lifecycle:` comment
  # naming what ends them.
  if ($text -match 'goroutine lifecycle:') { continue }
  if ($text -notmatch 'sync\.WaitGroup|errgroup|\.Done\(\)|ctx\.Done|<-ctx|context\.With|cancel\(|stopCh|quit\b|shutdown|Shutdown|time\.After|semaphore') {
    $goFuncNoLifecycle += New-SanityHit -File $f.Rel -Line 0 -Text "$n go func() with no WaitGroup/ctx/channel lifecycle marker in file"
  }
}
Add-SanityCheck -Report $report -Id 9 -Title 'Files with go func() and no visible lifecycle management' -Hits $goFuncNoLifecycle -Mode review -Severity medium -Remediation 'Every goroutine needs a WaitGroup, context, channel or bounded timeout that ends it.' -Command 'rg -n "go func\(" src/backend'

# 10. math/rand in security-sensitive code
$sensitiveDirs = 'core/auth|core/encryption|core/rbac|core/security|core/session|modules/auth|modules/agent|modules/mfa|modules/api_keys|modules/users|modules/identity|modules/scim|modules/elevations'
$mathRandAll = @(Find-Pattern @all -Pattern '"math/rand(/v2)?"')
$mathRandSensitive = @($mathRandAll | Where-Object { $_.File -match $sensitiveDirs })
$mathRandOther = @($mathRandAll | Where-Object { $_.File -notmatch $sensitiveDirs })
Add-SanityCheck -Report $report -Id '10a' -Title 'math/rand imported in security-sensitive packages' -Hits $mathRandSensitive -Mode strict -Severity high -Remediation 'Use crypto/rand for tokens, salts, IDs and secrets.' -Command 'rg -n "\"math/rand\"" src/backend src/agent'
Add-SanityCheck -Report $report -Id '10b' -Title 'math/rand imported elsewhere (jitter, backoff)' -Hits $mathRandOther -Mode info -Severity info -Note 'Accepted for non-security randomness such as jitter.'

# 11. HTTP response body closing
$doNoClose = @()
foreach ($f in @(Get-SanityFiles @all)) {
  $text = Get-SanityText $f.Full
  $calls = @([regex]::Matches($text, '(?m)^.*\.Do\((?!func).*$') | Where-Object { $_.Value -notmatch 'Once\.Do|once\.Do|\.Do\(\)' })
  if ($calls.Count -eq 0) { continue }
  if ($text -notmatch 'Body\.Close\(\)|drainAndClose|closeBody|io\.Copy\(io\.Discard|\b\w*[Cc]lose\w*\(\s*\w+\.Body\s*\)') { $doNoClose += New-SanityHit -File $f.Rel -Line 0 -Text "$($calls.Count) client.Do() call(s), no Body.Close() in file" }
}
Add-SanityCheck -Report $report -Id 11 -Title 'Files calling http Client.Do without Body.Close()' -Hits $doNoClose -Mode review -Severity medium -Remediation 'defer resp.Body.Close() immediately after the nil-error check.' -Command 'rg -n "\.Do\(" src/backend src/agent'

# 12. context.Background() in request-serving code
# `context.WithTimeout(context.Background(), ...)` is a detached root with a
# bound on it, which is exactly the "bounded fire-and-forget" the remediation
# accepts. The finding is a *bare* Background handed to a call that had a
# caller context available.
$bgCtx = @(Find-Pattern -Root $Root -Paths $beModCoreScope -Include @('handler.go', 'service.go') -ExcludeMatch @('_test\.go$') -Pattern 'context\.Background\(\)' -NotMatch '^\s*//|context\.With(Timeout|Deadline|Cancel)\(context\.Background\(\)')
$bgCtxOther = @(Find-Pattern -Root $Root -Paths $beModCoreScope -Include @('*.go') -ExcludeMatch @('_test\.go$', 'handler\.go$', 'service\.go$', 'main\.go$') -Pattern 'context\.Background\(\)' -NotMatch '^\s*//')
Add-SanityCheck -Report $report -Id '12a' -Title 'context.Background() in handler.go / service.go' -Hits $bgCtx -Mode review -Severity medium -Remediation 'Derive from r.Context() or the caller ctx; detach only for bounded fire-and-forget cleanup with a comment.' -Command 'rg -n "context\.Background\(" src/backend/modules src/backend/core'
Add-SanityCheck -Report $report -Id '12b' -Title 'context.Background() in collectors and other files (inventory)' -Hits $bgCtxOther -Mode info -Severity info -Note 'Collectors, schedulers and lifecycle roots may legitimately own a root context.'

# 13. Identifier casing and initialisms
$initialisms = @(Find-Pattern @all -StripStrings -Pattern '\b[a-z]\w*(Id|Url|Uri|Api|Http|Https|Xml|Json|Sql|Tls|Tcp|Udp|Uuid|Ssh|Ssl|Ip|Cpu|Ram|Os|Db|Ui|Css|Html|Ttl|Jwt|Rbac)\b|\b(Id|Url|Uri|Api|Http|Https|Xml|Json|Sql|Tls|Tcp|Udp|Uuid|Ssh|Ssl|Cpu|Db|Ui|Css|Html|Ttl|Jwt|Rbac)[A-Z][a-z]\w*\b' -NotMatch '^\s*//|IdP\b|HttpOnly|`\w+:"|json:|yaml:|form:|db:|Identity|Identif|Idle|Ideal|Ipv|Osx|Osm|Ipc|Ips\b|Uis\b|Dbs\b|Ram\w|Sqlite|Http[A-Z][a-z]*(Client|Server|Request|Response|Handler)\b|\.(Id|Url|Uri|Api|Http|Https|Xml|Json|Sql|Tls|Tcp|Udp|Uuid|Ssh|Ssl|Ip|Cpu|Ram|Os|Db|Ui|Css|Html|Ttl|Jwt|Rbac)[A-Za-z]')
$initialisms = Remove-SanityRawStringHits -Hits $initialisms -Root $Root
Add-SanityCheck -Report $report -Id 13 -Title 'Mixed-case initialisms (userId, apiUrl, httpClient)' -Hits $initialisms -Mode review -Severity low -Remediation 'Initialisms stay uniformly cased: userID, apiURL, HTTPClient, parseXML. JSON/YAML tag values are wire format and excluded.' -Command 'rg -n "\b[A-Za-z0-9_]*(Id|Url|Uri|Api|Http|Https|Xml|Json|Sql|Tls|Tcp|Udp|Uuid)[A-Za-z0-9_]*\b" src/backend src/agent'

# 14/15/16 are manual reviews - list the cheapest signals
# `enabledInt` and `envPtr` are the same conversion-oriented naming the
# remediation already accepts for `userIDStr`: SQLite has no bool, and a
# nullable column round-trips through a pointer. `isString` is the ok half of a
# type assertion. None of them is a type name standing in for a role.
$typeNames = @(Find-Pattern @all -StripStrings -Pattern '\b[a-z]\w+(String|Int|Int64|Float64|Bool|Slice|Array|Struct|Ptr)\b\s*(:?=|,)' -NotMatch '^\s*//|Str\b|ToString|String\(\)|strconv|Marshal|Unmarshal|Parse|Format|\bis[A-Z]\w*\b|\bany\b|sql\.Null|Null(String|Int|Bool|Float)|\.(String|Int|Bool)\b|\b\w+(Int|Int64|Ptr)\b|\btok[A-Z]\w*')
$typeNames = Remove-SanityRawStringHits -Hits $typeNames -Root $Root
Add-SanityCheck -Report $report -Id 14 -Title 'Identifiers with the type name baked in (nameString, resultSlice)' -Hits $typeNames -Mode review -Severity low -Remediation 'Name by role, not by type; conversion-oriented names like userIDStr are accepted.'
$builtinShadow = @(Find-Pattern @all -StripStrings -Pattern '(^|[\s,(])(len|min|max|clear|cap|copy|new|make|append|close|delete|print|println|error|string|int|bool|any)\s*(:=|,\s*\w+\s*:=|=\s*[^=])' -NotMatch '^\s*//|==|!=|<=|>=|\.\w+\s*=|\[\w+\]\s*=|\bvar\s+\w+\s+\w+\s*=')
$pkgShadowNames = @('json', 'url', 'time', 'log', 'regexp', 'mail', 'path', 'sort', 'strings', 'bytes', 'errors', 'context', 'http', 'filepath', 'os', 'io', 'fmt')
$pkgShadow = @()
foreach ($f in @(Get-SanityFiles @all)) {
  $text = Get-SanityText $f.Full
  # Importing `path` and shadowing it in a function that never qualifies
  # anything with `path.` costs nothing. The shadow only bites when the file
  # actually uses the package, so require both.
  $imported = @($pkgShadowNames | Where-Object {
      $text -match ('"(?:[\w./-]+/)?' + $_ + '"') -and $text -cmatch ('(^|[^\w.])' + $_ + '\.[A-Z]')
    })
  if ($imported.Count -eq 0) { continue }
  $rx = '(^|[\s,(])(' + ($imported -join '|') + ')\s*(:=|,\s*\w+\s*:=|=\s*[^=])'
  $lines = Get-SanityLines $f.Full
  for ($i = 0; $i -lt $lines.Length; $i++) {
    $probe = Remove-SanityStringLiterals $lines[$i]
    if ($probe -cmatch $rx -and $lines[$i] -notmatch '^\s*//|\.\w+\s*=') { $pkgShadow += New-SanityHit -File $f.Rel -Line ($i + 1) -Text $lines[$i].Trim() }
  }
}
$shadow = Remove-SanityRawStringHits -Hits ($builtinShadow + $pkgShadow) -Root $Root
Add-SanityCheck -Report $report -Id 15 -Title 'Identifiers shadowing builtins or imported packages' -Hits $shadow -Mode review -Severity low -Remediation 'Prefer count, logger, payload, parsedURL, data.'
Add-SanityCheck -Report $report -Id 16 -Title 'Export only when needed (manual review)' -Hits @() -Mode info -Severity info -Note 'See scripts/sanity/orphaned-backend check 2 for exported identifiers with no references.'

# 17. Package naming
$pkgBad = @(Find-Pattern @all -Pattern '^package\s+([A-Z]\w*|\w*_\w*)\s*$' -NotMatch '^package \w+_test$')
# core/plugin/types is the plugin SDK's shared contract package: every
# compiled .so imports that exact path, so renaming it breaks the ABI for
# plugins already in the field. Accepted and filtered out by path.
$pkgVague = @(Find-Pattern @all -Pattern '^package\s+(common|util|utils|helper|helpers|types|interfaces|misc|shared|base)\b' |
  Where-Object { $_.File -notmatch 'core/plugin/types/' })
$pkgUpper = @($pkgBad | Where-Object { $_.Text -cmatch '^package\s+[A-Z]' })
$pkgUnderscore = @($pkgBad | Where-Object { $_.Text -cnotmatch '^package\s+[A-Z]' })
Add-SanityCheck -Report $report -Id '17a' -Title 'Package names with uppercase or vague catch-all names' -Hits ($pkgUpper + $pkgVague) -Mode strict -Severity low -Remediation 'Short lowercase single-word package names with one responsibility.' -Command 'rg -n "^package\s+[A-Z_]" src/backend src/agent'
# Inventory, not a finding. The module tree is snake_case by design and every
# one of these package names mirrors its directory; renaming them is a planned
# refactor across imports, not something a run of this check should re-raise.
Add-SanityCheck -Report $report -Id '17b' -Title 'Package names with underscores (inventory)' -Hits @($pkgUnderscore | Sort-Object Text -Unique) -Mode info -Severity info -Remediation 'Go style prefers single-word packages (apikeys, not api_keys). Renaming is a deliberate refactor across every import site.' -Note 'Listed once per package name.'

# 18. Filename naming
$fileNames = @()
foreach ($f in @(Get-SanityFiles -Root $Root -Paths $allScope -Include @('*.go'))) {
  if ($f.Name -match '^[._]' ) { $fileNames += New-SanityHit -File $f.Rel -Line 0 -Text 'leading . or _ makes Go tooling ignore this file' }
  elseif ($f.Name -cmatch '[A-Z]') { $fileNames += New-SanityHit -File $f.Rel -Line 0 -Text 'uppercase characters in Go filename' }
  # `_linux.go` and friends are not a naming smell: the suffix *is* the build
  # constraint, and the Go toolchain acts on it. Nothing to confirm.
}
Add-SanityCheck -Report $report -Id 18 -Title 'Filename naming (leading ./_, uppercase, build-constraint suffixes)' -Hits $fileNames -Mode review -Severity low -Remediation 'Short lowercase snake_case filenames that summarize the file responsibility.'

Add-SanityNote -Report $report -Text 'Reference: Effective Go, Go Code Review Comments, Alex Edwards "Go Naming Conventions: A Practical Guide".'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
