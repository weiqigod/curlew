[CmdletBinding()]
param(
    [string]$GoCommand = "go",
    [string]$PythonCommand = "python"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Invoke-Checked {
    param([Parameter(Mandatory)][string]$Command, [Parameter(ValueFromRemainingArguments)][string[]]$Arguments)
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command exited $LASTEXITCODE"
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$marker = New-TemporaryFile
Remove-Item -LiteralPath $marker -Force
$smokeRoot = New-Item -ItemType Directory -Path $marker
$server = $null

try {
    $portFile = Join-Path $smokeRoot "httpbin.port"
    $fixture = Join-Path $PSScriptRoot "fixtures/httpbin_server.py"
    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $PythonCommand
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    foreach ($argument in @($fixture, "0", "--port-file", $portFile)) {
        $startInfo.ArgumentList.Add($argument)
    }
    $server = [Diagnostics.Process]::Start($startInfo)
    $server.BeginOutputReadLine()
    $server.BeginErrorReadLine()

    $env:CURLEW_CONFIG_DIR = Join-Path $smokeRoot "config"
    $env:CURLEW_TELEMETRY_FILE = Join-Path $smokeRoot "telemetry.ndjson"
    $env:CURLEW_PLUGINS = ""
    $env:CURLEW_TEAM_CONFIG = ""
    New-Item -ItemType Directory -Path $env:CURLEW_CONFIG_DIR | Out-Null

    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    while (-not (Test-Path -LiteralPath $portFile)) {
        if ($server.HasExited) { throw "loopback fixture exited $($server.ExitCode)" }
        if ([DateTime]::UtcNow -ge $deadline) { throw "loopback fixture did not publish its port" }
        [Threading.Thread]::Sleep(50)
    }
    $port = [int](Get-Content -LiteralPath $portFile -Raw)
    $baseUrl = "http://127.0.0.1:$port"

    $binary = Join-Path $smokeRoot "curlew.exe"
    Push-Location $repoRoot
    try {
        Invoke-Checked $GoCommand "build" "-buildvcs=false" "-o" $binary "./cmd/curlew"
    } finally {
        Pop-Location
    }

    $collection = Join-Path $smokeRoot "smoke.yaml"
    @"
name: Windows native smoke
requests:
  - name: loopback
    request:
      method: GET
      url: "$baseUrl/get?source=windows"
    assertions:
      status: 200
      body:
        `$.args.source:
          equals: windows
"@ | Set-Content -LiteralPath $collection -Encoding utf8NoBOM

    Invoke-Checked $binary "--version"
    Invoke-Checked $binary "validate" $collection "--format" "json"
    Invoke-Checked $binary "run" $collection "--format" "json"

    $bad = Join-Path $smokeRoot "bad.yaml"
    "requests: [" | Set-Content -LiteralPath $bad -Encoding utf8NoBOM
    & $binary validate $bad --format json *> $null
    if ($LASTEXITCODE -ne 3) {
        throw "invalid collection exited $LASTEXITCODE, want 3"
    }
    Write-Output "WINDOWS_SMOKE_PASS root=$smokeRoot url=$baseUrl"
} finally {
    if ($null -ne $server -and -not $server.HasExited) {
        Stop-Process -Id $server.Id -Force
        $server.WaitForExit()
    }
    Remove-Item -LiteralPath $smokeRoot -Recurse -Force -ErrorAction SilentlyContinue
}