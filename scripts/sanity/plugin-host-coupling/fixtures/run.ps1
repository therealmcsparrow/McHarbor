# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$check = Join-Path $PSScriptRoot '..\check.ps1'
$work = Join-Path ([System.IO.Path]::GetTempPath()) ('coupling-fixture-' + [guid]::NewGuid().ToString('N'))
$failures = 0

function New-FixtureRoot([string]$name, [string]$hostFile, [string]$dest) {
  $rootDir = Join-Path $work $name
  New-Item -ItemType Directory -Force -Path (Join-Path $rootDir 'src\plugins\Plugins\addon-demo\backend'), (Join-Path $rootDir 'src\backend\modules\example'), (Join-Path $rootDir 'src\frontend\modules\example'), (Split-Path -Parent (Join-Path $rootDir $dest)) | Out-Null
  Copy-Item (Join-Path $PSScriptRoot 'plugin.yaml.fixture') (Join-Path $rootDir 'src\plugins\Plugins\addon-demo\plugin.yaml')
  Copy-Item (Join-Path $PSScriptRoot 'scanner-main.go.fixture') (Join-Path $rootDir 'src\plugins\Plugins\addon-demo\backend\main.go')
  Copy-Item (Join-Path $PSScriptRoot $hostFile) (Join-Path $rootDir $dest)
  return $rootDir
}

try {
  $goRoute = 'src\backend\modules\example\route.go'
  $tsApi = 'src\frontend\modules\example\api.ts'
  $goSend = 'src\backend\modules\workflows\send.go'
  $tsNode = 'nodes\scan\index.ts'
  foreach ($case in @(
      @{ Name = 'literal'; File = 'literal.go.fixture'; Dest = $goRoute; Want = 1 },
      @{ Name = 'clean'; File = 'clean.go.fixture'; Dest = $goRoute; Want = 0 },
      @{ Name = 'client-literal'; File = 'client-literal.ts.fixture'; Dest = $tsApi; Want = 1 },
      @{ Name = 'client-clean'; File = 'client-clean.ts.fixture'; Dest = $tsApi; Want = 0 },
      @{ Name = 'channel-literal'; File = 'channel-literal.go.fixture'; Dest = $goSend; Want = 1 },
      @{ Name = 'channel-clean'; File = 'channel-clean.go.fixture'; Dest = $goSend; Want = 0 },
      @{ Name = 'engine-literal'; File = 'engine-literal.ts.fixture'; Dest = $tsNode; Want = 1 },
      @{ Name = 'bridge-host'; File = 'compute-bridge.go.fixture'; Dest = 'src\backend\core\example\bridge.go'; Want = 1 },
      @{ Name = 'bridge-plugin'; File = 'compute-bridge.go.fixture'; Dest = 'src\plugins\Plugins\addon-demo\backend\bridge.go'; Want = 1 },
      @{ Name = 'bridge-clean'; File = 'clean.go.fixture'; Dest = 'src\backend\core\example\bridge.go'; Want = 0 },
      @{ Name = 'kind-go'; File = 'kind-literal.go.fixture'; Dest = 'src\backend\modules\example\guest.go'; Want = 1 },
      @{ Name = 'kind-core'; File = 'kind-literal.go.fixture'; Dest = 'src\backend\core\example\guest.go'; Want = 1 },
      @{ Name = 'kind-tsx'; File = 'kind-literal.tsx.fixture'; Dest = 'src\frontend\resources\example\Params.tsx'; Want = 1 },
      @{ Name = 'kind-clean'; File = 'kind-clean.go.fixture'; Dest = $goRoute; Want = 0 },
      @{ Name = 'kind-exempt'; File = 'kind-literal.go.fixture'; Dest = $goRoute; Want = 0 },
      @{ Name = 'kind-agent'; File = 'kind-literal.go.fixture'; Dest = 'src\backend\core\agent\guest.go'; Want = 0 },
      @{ Name = 'kind-test'; File = 'kind-literal.go.fixture'; Dest = 'src\backend\modules\example\guest_test.go'; Want = 0 })) {
    $rootDir = New-FixtureRoot $case.Name $case.File $case.Dest
    & powershell -NoProfile -ExecutionPolicy Bypass -File $check -Root $rootDir -OutDir (Join-Path $work 'out') -AllowList (Join-Path $PSScriptRoot 'allow-kinds.json.fixture') | Out-Null
    $code = $LASTEXITCODE
    $verdict = if ($code -eq $case.Want) { 'ok' } else { $failures++; 'WRONG' }
    Write-Host ("{0,-15} exit {1} (want {2}) {3}" -f $case.Name, $code, $case.Want, $verdict)
  }
} finally {
  Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
if ($failures -gt 0) { exit 1 }
