# Pester 3+ compatible: Invoke-Pester scripts/release-notes.Tests.ps1
. "$PSScriptRoot\release-notes.ps1"

Describe 'Get-ChangelogSection' {
    $f = Join-Path $env:TEMP 'release-notes-test-changelog.md'
    Set-Content -LiteralPath $f -Encoding UTF8 -Value @('# Changelog', '', '## 1.1.0', 'new', '', '## 1.0.0', 'first', 'second', '', '## 0.9.0', 'old')
    It 'returns the body of the requested version only' {
        (Get-ChangelogSection $f '1.0.0') | Should Be "first`nsecond"
    }
    It 'does not match a version prefix' {
        (Get-ChangelogSection $f '1.0') | Should BeNullOrEmpty
    }
    It 'returns null for an unknown version' {
        (Get-ChangelogSection $f '2.0.0') | Should BeNullOrEmpty
    }
    It 'reads the last section up to the end of the file' {
        (Get-ChangelogSection $f '0.9.0') | Should Be 'old'
    }
    Remove-Item $f
}

Describe 'release-notes.ps1 as a script' {
    $script = Join-Path $PSScriptRoot 'release-notes.ps1'
    $f = Join-Path $env:TEMP 'release-notes-test-changelog2.md'
    $out = Join-Path $env:TEMP 'release-notes-test-out.md'
    $uk = -join [char[]](0x43D, 0x43E, 0x432, 0x435)   # Cyrillic, kept ASCII-only in this file
    Set-Content -LiteralPath $f -Encoding UTF8 -Value @('# Changelog', '', '## 1.1.0', $uk, '', '## 1.0.0', 'first')
    It 'writes the section to -OutFile as UTF-8 without BOM, LF, trailing newline' {
        & powershell -NoProfile -File $script -Version 1.1.0 -Path $f -OutFile $out
        $LASTEXITCODE | Should Be 0
        $bytes = [IO.File]::ReadAllBytes($out)
        ($bytes[0] -eq 0xEF) | Should Be $false
        [Text.Encoding]::UTF8.GetString($bytes) | Should Be "$uk`n"
    }
    It 'exits 1 for a missing version' {
        $err = Join-Path $env:TEMP 'release-notes-test-err.txt'
        $p = Start-Process powershell -ArgumentList @('-NoProfile', '-File', "`"$script`"", '-Version', '9.9.9', '-Path', "`"$f`"") -Wait -PassThru -NoNewWindow -RedirectStandardError $err
        $p.ExitCode | Should Be 1
        (Get-Content -Raw -LiteralPath $err) | Should Match "no non-empty .## 9.9.9. section"
        Remove-Item $err
    }
    It 'finds a section of the real CHANGELOG' {
        & powershell -NoProfile -File $script -Version 1.0.0 -OutFile $out
        $LASTEXITCODE | Should Be 0
        ((Get-Content -Raw -Encoding UTF8 -LiteralPath $out).Length -gt 0) | Should Be $true
    }
    Remove-Item $f, $out -ErrorAction SilentlyContinue
}
