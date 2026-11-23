<#
.SYNOPSIS
  EMERGENCY, WINDOWS-ONLY FALLBACK: builds and (after owner approval) publishes a Windows-only
  Marstek Monitor release from this PC.
.DESCRIPTION
  The normal release is .github/workflows/release.yml: push the tag v<version> and the workflow
  builds Windows and macOS and creates a draft in the releases repo (docs/releasing.md). Use this
  script only when the workflow cannot run. A release made here has no macOS dmg, so Mac users
  get no update offer for that version.

  Steps, failing fast: clean tree on the release branch; version not tagged; CHANGELOG has the
  version (scripts/release-notes.ps1); wails3 build (frontend deps, bindings and dist, which are
  gitignored); go vet / go test / go test -tags release ./core/update/ / npm test / typecheck;
  installer, whose exe must carry the `release` build tag (go version -m); SHA-256 file; local
  git tag; `gh release create`. With -DryRun everything except the tag and `gh release create`
  is done, and those two are only printed. Without -DryRun the script stops at a typed
  confirmation: publishing is the owner's decision, never an agent's.
.EXAMPLE
  scripts/release.ps1 -Version 1.0.0 -DryRun
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Version,
    [switch]$DryRun,
    [string]$Repo = 'econ11/marstek-monitor',
    [string]$Branch = 'main'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Fail([string]$Message) { Write-Host "release: $Message" -ForegroundColor Red; exit 1 }
function Step([string]$Message) { Write-Host "==> $Message" -ForegroundColor Cyan }
function Invoke-Checked([string]$What, [scriptblock]$Cmd) {
    & $Cmd
    if ($LASTEXITCODE -ne 0) { Fail "$What failed (exit $LASTEXITCODE)" }
}

if ($MyInvocation.InvocationName -eq '.') { return }   # dot-sourced by the tests: functions only

if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$') { Fail "version must look like 1.2.3 or 1.2.3-rc.1 (got '$Version')" }
$tag = "v$Version"
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Set-Location $root
if ($DryRun) { Write-Host 'DRY RUN: no tag is created and nothing is published.' -ForegroundColor Yellow }

Step '1/9 clean tree on the release branch'
$current = (git rev-parse --abbrev-ref HEAD).Trim()
if ($current -ne $Branch) { Fail "on branch '$current', expected '$Branch'" }
if (git status --porcelain) { Fail 'working tree is not clean; commit or stash first' }

Step "2/9 $tag is not tagged yet"
if (git tag --list $tag) { Fail "tag $tag already exists locally" }

Step "3/9 CHANGELOG has '## $Version'"
$notes = & (Join-Path $PSScriptRoot 'release-notes.ps1') -Version $Version -Path (Join-Path $root 'CHANGELOG.md')
if ($LASTEXITCODE -ne 0 -or -not $notes) { Fail "CHANGELOG.md has no non-empty '## $Version' section" }
$notes = (@($notes) -join "`n")

Step '4/9 prepare: wails3 build (npm install, bindings, frontend/dist; a fresh clone has none of them)'
Invoke-Checked 'wails3 build' { wails3 build }

Step '5/9 checks: go vet, go test, go test -tags release ./core/update/, npm test, typecheck'
Invoke-Checked 'go vet' { go vet ./... }
Invoke-Checked 'go test' { go test ./... }
Invoke-Checked 'go test -tags release' { go test -tags release ./core/update/ }
Invoke-Checked 'npm test' { npm --prefix frontend test }
Invoke-Checked 'typecheck' { npm --prefix frontend run typecheck }

Step "6/9 installer (wails3 task package VERSION=$Version)"
if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) {
    $nsis = @("${env:ProgramFiles(x86)}\NSIS", "$env:ProgramFiles\NSIS") |
        Where-Object { $_ -and (Test-Path (Join-Path $_ 'makensis.exe')) } | Select-Object -First 1
    if (-not $nsis) { Fail 'makensis not found: install NSIS 3 (https://nsis.sourceforge.io) or add it to PATH' }
    $env:PATH = "$nsis;$env:PATH"
}
Invoke-Checked 'package' { wails3 task package "VERSION=$Version" }
$installer = Join-Path $root "bin\marstek-monitor-$Version-setup.exe"
if (-not (Test-Path $installer)) { Fail "installer not produced: $installer" }
# The installer ships bin\marstek-monitor.exe; a release build keeps the releases repo pinned.
$exe = Join-Path $root 'bin\marstek-monitor.exe'
$buildInfo = go version -m $exe
if ($LASTEXITCODE -ne 0) { Fail "go version -m failed on $exe" }
if (-not ($buildInfo | Where-Object { $_ -match '^\s*build\s+-tags=(\S*,)?release(,|\s|$)' })) {
    Fail "the shipped exe lacks the 'release' build tag (go version -m $exe)"
}
Write-Host '    the exe carries the release build tag'

Step '7/9 SHA-256'
$hash = (Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToLowerInvariant()
$sumFile = "$installer.sha256"
# LF, no BOM: `<hash>  <filename>` as sha256sum prints it
[IO.File]::WriteAllText($sumFile, "$hash  $(Split-Path $installer -Leaf)`n", (New-Object Text.UTF8Encoding($false)))
$notesFile = Join-Path $root "bin\release-notes-$Version.md"
[IO.File]::WriteAllText($notesFile, "$notes`n", (New-Object Text.UTF8Encoding($false)))
Write-Host "    $hash"

Step '8/9 re-check the tree after the build'
# Content comparison, not `git status`: with core.autocrlf=true, a tool that rewrites a file with
# the same LF content (go mod tidy on go.mod in a fresh clone) shows as " M" in status while
# `git diff` correctly finds nothing.
git diff --quiet HEAD --
if ($LASTEXITCODE -ne 0) { Fail 'tracked files changed during the build; review `git diff`, commit, and rerun' }

Step '9/9 gh release create, then the local tag'
$pre = if ($Version -match '-') { @('--prerelease') } else { @() }
$ghArgs = @('release', 'create', $tag, '--repo', $Repo, '--title', "Marstek Monitor $Version",
    '--notes-file', $notesFile) + $pre + @($installer, $sumFile)
Write-Host ("    gh " + (($ghArgs | ForEach-Object { if ($_ -match "\s") { "`"$_`"" } else { $_ } }) -join ' '))
if ($DryRun) {
    Write-Host "    then, only after gh succeeded: git tag -a $tag -m `"Marstek Monitor $Version`""
    Write-Host 'DRY RUN complete: all checks passed, gh not run.' -ForegroundColor Green
    exit 0
}

$answer = Read-Host "Publish $tag to $Repo? This is public. Type 'publish $tag' to confirm"
if ($answer -ne "publish $tag") { Fail 'not confirmed; nothing published, no tag created' }
gh @ghArgs
if ($LASTEXITCODE -ne 0) { Fail "gh release create failed (exit $LASTEXITCODE); no local tag was created. Check 'gh release list --repo $Repo' before retrying" }
git tag -a $tag -m "Marstek Monitor $Version"
if ($LASTEXITCODE -ne 0) { Fail "published, but the local tag could not be created; run: git tag -a $tag -m `"Marstek Monitor $Version`"" }
Write-Host "Published $tag." -ForegroundColor Green
