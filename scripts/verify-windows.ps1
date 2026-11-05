[CmdletBinding()]
param(
    [switch]$PreflightOnly,
    [Alias("Profile")]
    [ValidateSet("Local", "Race", "Release", "Full")]
    [string]$VerificationProfile = "Local",
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

$runLocal = $VerificationProfile -in @("Local", "Full")
$runRace = $VerificationProfile -in @("Race", "Full")
$runRelease = $VerificationProfile -in @("Release", "Full")
$savedEnvironment = @{}
foreach ($name in @("GOROOT", "PATH", "CGO_ENABLED", "CC")) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
}

try {
    if ($runLocal -or $runRace) {
        $go = Get-RequiredTool $GoCommand
        $env:GOROOT = Split-Path -Parent (Split-Path -Parent $go)
        $env:PATH = (Split-Path -Parent $go) + ";" + $env:PATH
        $goVersion = & $go version
        if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch 'go1\.(\d+)') {
            throw "could not read Go version: $goVersion"
        }
        if ([int]$Matches[1] -lt 24) {
            throw "Go 1.24 or newer is required; got $goVersion"
        }
    }

    if ($runLocal) {
        $node = Get-RequiredTool $NodeCommand
        $npm = Get-RequiredTool $NpmCommand
        $python = Get-RequiredTool $PythonCommand
        $nodeVersion = & $node --version
        if ($LASTEXITCODE -ne 0 -or $nodeVersion -notmatch '^v(\d+)\.') {
            throw "could not read Node.js version: $nodeVersion"
        }
        if ([int]$Matches[1] -lt 22) {
            throw "Node.js 22 or newer is required; got $nodeVersion"
        }
        Invoke-Checked $npm "--version"
        $pythonVersion = (& $python --version 2>&1 | Out-String).Trim()
        if ($LASTEXITCODE -ne 0 -or $pythonVersion -notmatch '^Python 3\.') {
            throw "Python 3 is required; got $pythonVersion"
        }

        $lint = Get-RequiredTool $LintCommand
        $lintGo = Get-RequiredTool $LintGoCommand
        $lintVersion = (& $lint version 2>&1 | Out-String)
        if ($LASTEXITCODE -ne 0 -or $lintVersion -notmatch '\b2\.11\.2\b') {
            throw "golangci-lint 2.11.2 is required; got $($lintVersion.Trim())"
        }
        $lintGoVersion = (& $lintGo version 2>&1 | Out-String).Trim()
        if ($LASTEXITCODE -ne 0 -or $lintGoVersion -notmatch 'go1\.26\.') {
            throw "Go 1.26.x is required for golangci-lint; got $lintGoVersion"
        }
    }

    if ($runRace) {
        $cCompiler = Get-RequiredTool $CCompilerCommand
        $compilerVersion = (& $cCompiler --version 2>&1 | Out-String).Trim()
        if ($LASTEXITCODE -ne 0 -or $compilerVersion -eq "") {
            throw "could not run C compiler: $cCompiler"
        }
    }

    if ($runRelease) {
        $goreleaser = Get-RequiredTool $GoReleaserCommand
        $goreleaserVersion = (& $goreleaser --version 2>&1 | Out-String)
        if ($LASTEXITCODE -ne 0 -or $goreleaserVersion -notmatch '(?m)^GitVersion:\s+v?2\.17\.1\s*$') {
            throw "GoReleaser 2.17.1 is required; got $($goreleaserVersion.Trim())"
        }
    }

    if ($PreflightOnly) {
        Write-Output "WINDOWS_PREFLIGHT_PASS profile=$VerificationProfile"
        return
    }

    $repoRoot = Split-Path -Parent $PSScriptRoot
    Push-Location $repoRoot
    try {
        if ($runLocal) {
            $env:CGO_ENABLED = "0"
            Invoke-Checked $npm "--prefix" "ui" "ci" "--no-fund"
            Invoke-Checked $npm "--prefix" "ui" "run" "check"
            Invoke-Checked $npm "--prefix" "ui" "run" "lint"
            Invoke-Checked $npm "--prefix" "ui" "test"
            Invoke-Checked $npm "--prefix" "ui" "run" "build"
            Invoke-Checked $go "build" "./cmd/curlew"
            Invoke-Checked $go "test" "./internal/backlog/" "-run" "^TestBacklog_repository_is_consistent$" "-count=1" "-v"
            Invoke-Checked $go "test" "-p" "1" "./..." "-count=1" "-timeout=30m"
            Invoke-Checked $go "test" "-p" "1" "-coverprofile=coverage-windows.out" "./..." "-timeout=30m"
            $coverageOutput = & $go tool cover "-func=coverage-windows.out"
            if ($LASTEXITCODE -ne 0) {
                throw "go tool cover exited $LASTEXITCODE"
            }
            $coverageOutput | Write-Output
            $totalCoverage = $coverageOutput | Select-String '^total:' | Select-Object -Last 1
            if ($null -eq $totalCoverage -or $totalCoverage.Line -notmatch '([0-9]+(?:\.[0-9]+)?)%') {
                throw "could not read total coverage"
            }
            $coveragePercent = [double]::Parse($Matches[1], [Globalization.CultureInfo]::InvariantCulture)
            if ($coveragePercent -lt 80) {
                throw "coverage is below 80 percent: $coveragePercent%"
            }
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
            Invoke-Checked "pwsh" "-NoProfile" "-File" (Join-Path $repoRoot "smoke/run.ps1") "-GoCommand" $go "-PythonCommand" $python
        }
        if ($runRace) {
            $env:CC = $cCompiler
            $env:CGO_ENABLED = "1"
            Invoke-Checked $go "test" "-p" "1" "-race" "./..." "-count=1" "-timeout=40m"
        }
        if ($runRelease) {
            Invoke-Checked $goreleaser "check"
        }
        switch ($VerificationProfile) {
            "Local" { Write-Output "WINDOWS_LOCAL_VERIFY_PASS" }
            "Race" { Write-Output "WINDOWS_RACE_VERIFY_PASS" }
            "Release" { Write-Output "WINDOWS_RELEASE_VERIFY_PASS" }
            "Full" { Write-Output "WINDOWS_VERIFY_PASS" }
        }
    } finally {
        if ($runLocal) {
            Remove-Item -LiteralPath (Join-Path $repoRoot "coverage-windows.out") -Force -ErrorAction SilentlyContinue
        }
        Pop-Location
    }
} finally {
    foreach ($name in $savedEnvironment.Keys) {
        if ($null -eq $savedEnvironment[$name]) {
            Remove-Item "Env:$name" -ErrorAction SilentlyContinue
        } else {
            [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], "Process")
        }
    }
}
