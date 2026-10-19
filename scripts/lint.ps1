$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
exit $LASTEXITCODE
