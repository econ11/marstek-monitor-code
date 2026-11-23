# Pester 3+ compatible: Invoke-Pester scripts/release.Tests.ps1
# The changelog parser is tested in release-notes.Tests.ps1.

Describe 'release.ps1 ordering' {
    $text = Get-Content -Raw -LiteralPath "$PSScriptRoot\release.ps1"
    It 'creates the real tag only after gh release create' {
        $gh = $text.IndexOf('gh @ghArgs')
        $tagAt = $text.IndexOf('git tag -a $tag -m "Marstek Monitor $Version"', $gh)
        ($gh -gt 0) | Should Be $true
        ($tagAt -gt $gh) | Should Be $true
    }
}

Describe 'release.ps1 checks' {
    $text = Get-Content -Raw -LiteralPath "$PSScriptRoot\release.ps1"
    It 'takes the notes from release-notes.ps1' {
        $text.Contains("'release-notes.ps1'") | Should Be $true
        $text.Contains('function Get-ChangelogSection') | Should Be $false
    }
    It 'runs the release-tag tests of core/update' {
        $text.Contains('go test -tags release ./core/update/') | Should Be $true
    }
    It 'checks the shipped exe for the release build tag after packaging, before hashing' {
        $pkg = $text.IndexOf('wails3 task package')
        $check = $text.IndexOf('go version -m $exe')
        $hash = $text.IndexOf('Get-FileHash')
        ($pkg -gt 0) | Should Be $true
        ($check -gt $pkg) | Should Be $true
        ($hash -gt $check) | Should Be $true
    }
    It 'says it is the emergency Windows-only fallback' {
        $text.Contains('EMERGENCY, WINDOWS-ONLY FALLBACK') | Should Be $true
    }
}
