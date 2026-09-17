[CmdletBinding()]
param(
    [switch]$PreflightOnly,
    [string]$GoCommand = "go",
    [string]$NodeCommand = "node",
    [string]$NpmCommand = "npm.cmd",
    [string]$PythonCommand = "python",
    [string]$LintCommand = "golangci-lint",
    [string]$LintGoCommand = "go",
    [string]$GoReleaserCommand = "goreleaser",
    [string]$CCompilerCommand = "gcc"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Get-RequiredTool {
    param([Parameter(Mandatory)][string]$Name)
    $tool = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -eq $tool) {
        throw "missing required tool: $Name"
    }
    return $tool.Source
}

function Invoke-Checked {
    param([Parameter(Mandatory)][string]$Command, [Parameter(ValueFromRemainingArguments)][string[]]$Arguments)
    Write-Host "=== $Command $($Arguments -join ' ') ==="
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command exited $LASTEXITCODE"
    }
}

$go = Get-RequiredTool $GoCommand
$goRoot = Split-Path -Parent (Split-Path -Parent $go)
$env:GOROOT = $goRoot
$env:PATH = (Split-Path -Parent $go) + ";" + $env:PATH
$node = Get-RequiredTool $NodeCommand
$npm = Get-RequiredTool $NpmCommand
$python = Get-RequiredTool $PythonCommand

$goVersion = & $go version
if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch 'go1\.(\d+)') {
    throw "could not read Go version: $goVersion"
}
if ([int]$Matches[1] -lt 24) {
    throw "Go 1.24 or newer is required; got $goVersion"
}
$nodeVersion = & $node --version
if ($LASTEXITCODE -ne 0 -or $nodeVersion -notmatch '^v(\d+)\.') {
    throw "could not read Node.js version: $nodeVersion"
}
if ([int]$Matches[1] -lt 22) {
    throw "Node.js 22 or newer is required; got $nodeVersion"
}
Invoke-Checked $npm "--version"
Invoke-Checked $python "--version"

if ($PreflightOnly) {
    Write-Output "WINDOWS_PREFLIGHT_PASS go=$goVersion node=$nodeVersion"
    exit 0
}

$lint = Get-RequiredTool $LintCommand
$lintGo = Get-RequiredTool $LintGoCommand
$goreleaser = Get-RequiredTool $GoReleaserCommand
$cCompiler = Get-RequiredTool $CCompilerCommand
$repoRoot = Split-Path -Parent $PSScriptRoot

Push-Location $repoRoot
try {
    Invoke-Checked $npm "--prefix" "ui" "ci" "--no-fund"
    Invoke-Checked $npm "--prefix" "ui" "run" "check"
    Invoke-Checked $npm "--prefix" "ui" "test"
    Invoke-Checked $npm "--prefix" "ui" "run" "build"
    Invoke-Checked $go "build" "./cmd/curlew"
    Invoke-Checked $go "test" "./internal/backlog/" "-run" "^TestBacklog_repository_is_consistent$" "-count=1" "-v"
    Invoke-Checked $go "test" "-p" "1" "./..." "-count=1" "-timeout=30m"
    $env:CC = $cCompiler
    $env:CGO_ENABLED = "1"
    Invoke-Checked $go "test" "-p" "1" "-race" "./..." "-count=1" "-timeout=40m"
    Invoke-Checked $go "test" "-p" "1" "-coverprofile=coverage-windows.out" "./..." "-timeout=30m"
    Invoke-Checked $go "tool" "cover" "-func=coverage-windows.out"
    $savedPath = $env:PATH
    $savedGoRoot = $env:GOROOT
    try {
        $env:GOROOT = Split-Path -Parent (Split-Path -Parent $lintGo)
        $env:PATH = (Split-Path -Parent $lintGo) + ";" + $savedPath
        Invoke-Checked $lint "run" "--timeout=10m"
    } finally {
        $env:PATH = $savedPath
        $env:GOROOT = $savedGoRoot
    }
    Invoke-Checked $goreleaser "check"
    Invoke-Checked "pwsh" "-NoProfile" "-File" (Join-Path $repoRoot "smoke/run.ps1") "-GoCommand" $go "-PythonCommand" $python
    Write-Output "WINDOWS_VERIFY_PASS"
} finally {
    Remove-Item -LiteralPath (Join-Path $repoRoot "coverage-windows.out") -Force -ErrorAction SilentlyContinue
    Pop-Location
}
