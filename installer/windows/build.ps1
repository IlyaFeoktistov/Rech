# Builds rech.exe and packages it into rech-setup.exe (NSIS). Run from the
# repository root on Windows (or via WSL: powershell.exe -File
# installer\windows\build.ps1), with Go and NSIS (makensis) in PATH.
# NSIS: https://nsis.sourceforge.io/ or "winget install NSIS.NSIS".
#
# ASCII-only on purpose: legacy Windows PowerShell (powershell.exe, as
# opposed to cross-platform pwsh.exe) reads a .ps1 file without a BOM using
# the system codepage, not UTF-8 - Cyrillic text here would come out
# garbled and fail to parse.
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Push-Location $root
try {
    Write-Host "Building rech.exe..."
    go build -o installer\windows\rech.exe .\cmd\rech

    $makensis = Get-Command makensis -ErrorAction SilentlyContinue
    if (-not $makensis) {
        $default = "C:\Program Files (x86)\NSIS\makensis.exe"
        if (Test-Path $default) {
            $makensis = $default
        } else {
            throw "makensis not found in PATH - install NSIS (winget install NSIS.NSIS) and retry"
        }
    } else {
        $makensis = $makensis.Source
    }

    Write-Host "Building rech-setup.exe..."
    Push-Location installer\windows
    try {
        & $makensis setup.nsi
    } finally {
        Pop-Location
    }
    Write-Host "Done: rech-setup.exe"
} finally {
    Pop-Location
}
