package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The real (not elevated) Windows PowerShell reads a psQuote literal sent
// through -EncodedCommand back as the exact path: no quote character, $ or
// $(…) in it escapes into code. No UAC, no firewall change.
func TestPowerShellReadsQuotedPath(t *testing.T) {
	if testing.Short() {
		t.Skip("starts powershell.exe")
	}
	out, err := powerShell("Write-Output " + psQuote(trickyExe))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimRight(string(out), "\r\n"); got != trickyExe {
		t.Errorf("PowerShell read %q, want %q", got, trickyExe)
	}
}

// The elevated script's NetSecurity import resolves on this Windows: the
// real (not elevated) PowerShell loads the module from $PSHOME, and its
// cmdlet then comes from that file. No UAC, no firewall change.
func TestPowerShellImportsNetSecurityFromSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("starts powershell.exe")
	}
	out, err := powerShell("$ErrorActionPreference = 'Stop'\n" + netSecurityImport + "\n" +
		"Write-Output (Get-Command New-NetFirewallRule).Module.Path")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimRight(string(out), "\r\n")
	sys, _ := windows.GetSystemDirectory()
	if !strings.EqualFold(got, filepath.Join(sys, `WindowsPowerShell\v1.0\Modules\NetSecurity\NetSecurity.psd1`)) {
		t.Errorf("New-NetFirewallRule comes from %q, want NetSecurity.psd1 under %q", got, sys)
	}
}

// Both the diagnosis and the elevated run start PowerShell by its full path
// under the system directory, never "powershell.exe" looked up on PATH.
func TestSystemPowerShellPath(t *testing.T) {
	ps, dir, err := systemPowerShell()
	if err != nil {
		t.Fatal(err)
	}
	sys, _ := windows.GetSystemDirectory()
	if !filepath.IsAbs(ps) || dir != sys || !strings.EqualFold(filepath.Dir(filepath.Dir(filepath.Dir(ps))), sys) ||
		!strings.EqualFold(filepath.Base(ps), "powershell.exe") {
		t.Errorf("powershell = %q in %q, want under %q", ps, dir, sys)
	}
	if _, err := os.Stat(ps); err != nil {
		t.Errorf("not there: %v", err)
	}
}

func TestPowerShellPathIsAbsolute(t *testing.T) {
	got := powerShellPath(`C:\Windows\system32`)
	if got != `C:\Windows\system32\WindowsPowerShell\v1.0\powershell.exe` || !filepath.IsAbs(got) {
		t.Errorf("powerShellPath = %q", got)
	}
}
