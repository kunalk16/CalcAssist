<#
.SYNOPSIS
  Build calcassist (Windows-friendly). Produces a static, CGO-free binary.

.EXAMPLE
  ./build.ps1                      # build for the current OS into ./bin
  ./build.ps1 -All                 # cross-compile windows/linux/darwin (amd64+arm64) into ./dist
  ./build.ps1 -Version 1.2.3       # stamp an explicit version
#>
param(
    [string]$Version = "0.1.0",
    [switch]$All
)

$ErrorActionPreference = "Stop"
$env:CGO_ENABLED = "0"

# Best-effort commit + date stamps.
$commit = "unknown"
try { $commit = (git rev-parse --short HEAD) 2>$null } catch {}
if (-not $commit) { $commit = "unknown" }
$date = (Get-Date -Format "yyyy-MM-dd")

$pkg = "calcassist/internal/version"
$ldflags = "-s -w -X '$pkg.Version=$Version' -X '$pkg.Commit=$commit' -X '$pkg.Date=$date'"
$main = "./cmd/calcassist"

function Build($goos, $goarch, $outDir) {
    $ext = ""
    if ($goos -eq "windows") { $ext = ".exe" }
    $out = Join-Path $outDir "calcassist-$goos-$goarch$ext"
    Write-Host "Building $goos/$goarch -> $out"
    $env:GOOS = $goos
    $env:GOARCH = $goarch
    go build -trimpath -ldflags $ldflags -o $out $main
    if ($LASTEXITCODE -ne 0) { throw "build failed for $goos/$goarch" }
}

if ($All) {
    New-Item -ItemType Directory -Force -Path dist | Out-Null
    Build "windows" "amd64" "dist"
    Build "windows" "arm64" "dist"
    Build "linux"   "amd64" "dist"
    Build "linux"   "arm64" "dist"
    Build "darwin"  "amd64" "dist"
    Build "darwin"  "arm64" "dist"
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    Write-Host "Done. Artifacts in ./dist"
} else {
    New-Item -ItemType Directory -Force -Path bin | Out-Null
    $ext = ""
    if ($IsWindows -or $env:OS -eq "Windows_NT") { $ext = ".exe" }
    $out = Join-Path "bin" "calcassist$ext"
    Write-Host "Building host -> $out"
    go build -trimpath -ldflags $ldflags -o $out $main
    if ($LASTEXITCODE -ne 0) { throw "build failed" }
    Write-Host "Done. Binary at $out"
}
