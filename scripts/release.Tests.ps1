# Pester 3+ compatible: Invoke-Pester scripts/release.Tests.ps1
. "$PSScriptRoot\release.ps1" -Version 0.0.0

Describe 'Get-ChangelogSection' {
    $f = Join-Path $env:TEMP 'release-test-changelog.md'
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
    Remove-Item $f
}

Describe 'release.ps1 ordering' {
    $text = Get-Content -Raw -LiteralPath "$PSScriptRoot\release.ps1"
    It 'creates the real tag only after gh release create' {
        $gh = $text.IndexOf('gh @ghArgs')
        $tagAt = $text.IndexOf('git tag -a $tag -m "Marstek Monitor $Version"', $gh)
        ($gh -gt 0) | Should Be $true
        ($tagAt -gt $gh) | Should Be $true
    }
}
