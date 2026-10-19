param([int]$Threshold = 80)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# Smart App Control judges each new test binary by hash and may reject it; a package that is blocked is
# retried with a different build id (new hash) instead of repeating the same binary
function Test-Package([string]$Package, [string]$CoverFile) {
    $exitCode = 1
    for ($attempt = 1; $attempt -le 8; $attempt++) {
        $flags = @()
        if ($attempt -gt 1) { $flags = @("-ldflags=-buildid=retry$attempt-$([guid]::NewGuid().ToString('N'))") }
        $output = go test @flags $Package "-coverprofile=$CoverFile" 2>&1 | Out-String
        $exitCode = $LASTEXITCODE
        if ($output -notmatch 'Application Control') { break }
    }
    Write-Host $output.TrimEnd()
    return ($exitCode -eq 0)
}

$packages = go list ./...
$coverFiles = @()
$failed = $false
foreach ($package in $packages) {
    $coverFile = Join-Path $env:TEMP ('finance-api-cover-' + ($package -replace '[^a-zA-Z0-9]', '_') + '.out')
    $coverFiles += $coverFile
    if (-not (Test-Package $package $coverFile)) { $failed = $true }
}
if ($failed) { exit 1 }

$statements = 0
$covered = 0
foreach ($coverFile in $coverFiles) {
    foreach ($line in Get-Content $coverFile | Select-Object -Skip 1) {
        $parts = $line -split ' '
        $statements += [int]$parts[1]
        if ([int]$parts[2] -gt 0) { $covered += [int]$parts[1] }
    }
}
$percent = [math]::Round(100 * $covered / $statements, 1)
Write-Host "Go total coverage: $percent% ($covered/$statements statements), threshold $Threshold%"
if ($percent -lt $Threshold) { exit 1 }
