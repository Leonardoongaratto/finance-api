$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

$unformatted = go run cmd/gofmt -l cmd internal
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
if ($unformatted) {
    Write-Output 'gofmt: files need formatting (run: go run cmd/gofmt -w cmd internal)'
    Write-Output $unformatted
    exit 1
}
Write-Output 'gofmt: ok'
