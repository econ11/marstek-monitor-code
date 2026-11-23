<#
.SYNOPSIS
  Prints (or writes) the release notes of one version: the body of its `## <version>` section in
  CHANGELOG.md.
.DESCRIPTION
  Shared by scripts/release.ps1 and .github/workflows/release.yml. Exits 1 when the section is
  missing or empty. -OutFile writes UTF-8 without BOM, LF, with a trailing newline.
  Works on Windows PowerShell 5.1 and PowerShell 7 (the CI runs it on Linux).
.EXAMPLE
  scripts/release-notes.ps1 -Version 1.1.0 -OutFile bin/release-notes-1.1.0.md
#>
[CmdletBinding()]
param(
    [string]$Version,
    [string]$Path,     # default: CHANGELOG.md next to scripts/
    [string]$OutFile
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Returns the body of the `## <version>` section of a changelog (without the heading), or $null.
function Get-ChangelogSection([string]$Path, [string]$Ver) {
    $lines = @(Get-Content -LiteralPath $Path -Encoding UTF8)
    $start = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match ('^##\s+' + [regex]::Escape($Ver) + '(\s|$)')) { $start = $i + 1; break }
    }
    if ($start -lt 0) { return $null }
    $end = $lines.Count
    for ($i = $start; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match '^##\s') { $end = $i; break }
    }
    if ($end -le $start) { return '' }
    return (($lines[$start..($end - 1)]) -join "`n").Trim()
}

if ($MyInvocation.InvocationName -eq '.') { return }   # dot-sourced by the tests: functions only

if (-not $Path) { $Path = Join-Path (Split-Path -Parent $PSScriptRoot) 'CHANGELOG.md' }
if (-not $Version) { [Console]::Error.WriteLine('release-notes: -Version is required'); exit 1 }
if (-not (Test-Path -LiteralPath $Path)) { [Console]::Error.WriteLine("release-notes: no changelog at $Path"); exit 1 }
$notes = Get-ChangelogSection $Path $Version
if (-not $notes) { [Console]::Error.WriteLine("release-notes: CHANGELOG has no non-empty '## $Version' section"); exit 1 }
if ($OutFile) {
    $full = [IO.Path]::GetFullPath([IO.Path]::Combine((Get-Location).Path, $OutFile))
    [IO.File]::WriteAllText($full, "$notes`n", (New-Object Text.UTF8Encoding($false)))
} else {
    Write-Output $notes
}
exit 0
