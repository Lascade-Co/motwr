# Cross-compiles motwr into dist/ for Windows and macOS.
# Pure-Go build (no cgo), so GOOS/GOARCH cross-compilation just works.
# Assets are NOT embedded, so each target ships a copy of assets/ next to the binary.
#
# Usage:  pwsh scripts/build-dist.ps1     (run from the repo root)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Set-Location $root

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Bin = "motwr.exe"; Label = "motwr-windows-amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Bin = "motwr";     Label = "motwr-macos-arm64"   },  # Apple Silicon
    @{ GOOS = "darwin";  GOARCH = "amd64"; Bin = "motwr";     Label = "motwr-macos-amd64"   }   # Intel Mac
)

if (Test-Path dist) { Remove-Item dist -Recurse -Force }

$env:CGO_ENABLED = "0"
foreach ($t in $targets) {
    $out = "dist/$($t.Label)"
    New-Item -ItemType Directory -Force -Path $out | Out-Null
    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    Write-Host "building $($t.Label) ..."
    go build -trimpath -ldflags="-s -w" -o "$out/$($t.Bin)" ./cmd/motwr
    Copy-Item assets "$out/assets" -Recurse
}

Write-Host "`nDone. Artifacts in dist/:"
Get-ChildItem dist -Directory | ForEach-Object { Write-Host "  $($_.Name)" }
