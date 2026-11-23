package app

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseNetProfilesPublic(t *testing.T) {
	for _, c := range []struct {
		file string
		want bool
	}{
		{"netprofile_public.json", true},   // one Private, one Public (0)
		{"netprofile_private.json", false}, // Private (1) and Domain (2)
		{"netprofile_single.json", true},   // one object, not an array
	} {
		got, err := parseNetProfiles(fixture(t, c.file))
		if err != nil || got != c.want {
			t.Errorf("%s: public = %v, %v; want %v", c.file, got, err, c.want)
		}
	}
	// No connected network: PowerShell prints nothing.
	if got, err := parseNetProfiles([]byte("  \r\n")); err != nil || got {
		t.Errorf("empty: public = %v, %v; want false", got, err)
	}
	// PowerShell 7 may name the category.
	if got, err := parseNetProfiles([]byte(`{"InterfaceAlias":"Wi-Fi","NetworkCategory":"Public"}`)); err != nil || !got {
		t.Errorf("named category: public = %v, %v; want true", got, err)
	}
	if _, err := parseNetProfiles([]byte("Get-NetConnectionProfile : error")); err == nil {
		t.Error("not JSON: want an error")
	}
}

func TestParseFirewallMayBlock(t *testing.T) {
	on, off := []byte(`{"Enabled":1}`), []byte(`{"Enabled":0}`)
	for _, c := range []struct {
		name          string
		profile, rule []byte
		want          bool
	}{
		{"enabled, no rule", on, fixture(t, "fwrule_none.json"), true},
		{"enabled, rule", on, fixture(t, "fwrule_present.json"), false},
		{"enabled, rule switched off", on, fixture(t, "fwrule_disabled.json"), true},
		{"profile off, no rule", off, fixture(t, "fwrule_none.json"), false},
		{"profile as bool", []byte(`[{"Enabled":true}]`), nil, true},
	} {
		got, err := parseFirewall(c.profile, c.rule)
		if err != nil || got != c.want {
			t.Errorf("%s: mayBlock = %v, %v; want %v", c.name, got, err, c.want)
		}
	}
	if _, err := parseFirewall([]byte(""), nil); err == nil {
		t.Error("no profile: want an error")
	}
}

func mustNet(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	n.IP = ip
	return n
}

func TestPrivateLANAndSubnets(t *testing.T) {
	home := mustNet(t, "192.168.50.20/24")
	cgnat := mustNet(t, "100.72.1.5/10") // a mobile hotspot's carrier address
	if hasPrivateLAN([]*net.IPNet{cgnat}) {
		t.Error("carrier-grade NAT only: want no LAN")
	}
	if !hasPrivateLAN([]*net.IPNet{cgnat, home}) || !hasPrivateLAN([]*net.IPNet{mustNet(t, "10.0.0.7/8")}) {
		t.Error("192.168.x / 10.x: want a LAN")
	}
	if inSubnets(net.ParseIP("192.168.50.50"), nil) {
		t.Error("no interfaces: want not in a subnet")
	}
	if !inSubnets(net.ParseIP("192.168.50.50"), []*net.IPNet{cgnat, home}) {
		t.Error("192.168.50.50 is on 192.168.50.0/24")
	}
	if inSubnets(net.ParseIP("192.168.1.47"), []*net.IPNet{home}) {
		t.Error("192.168.1.47 is not on 192.168.50.0/24")
	}
}

// psLiteral reads the PowerShell single-quoted literal at s[0] the way
// PowerShell does (any of the 5 single-quote characters opens and closes
// it; two in a row are one): the value and the rest of s after it.
func psLiteral(t *testing.T, s string) (val, rest string) {
	t.Helper()
	quote := func(r rune) bool { return strings.ContainsRune("'‘’‚‛", r) }
	rs := []rune(s)
	if len(rs) == 0 || !quote(rs[0]) {
		t.Fatalf("no literal at %q", s)
	}
	var b strings.Builder
	for i := 1; i < len(rs); i++ {
		if quote(rs[i]) {
			if i+1 < len(rs) && quote(rs[i+1]) {
				b.WriteRune(rs[i])
				i++
				continue
			}
			return b.String(), string(rs[i+1:])
		}
		b.WriteRune(rs[i])
	}
	t.Fatalf("literal not closed: %q", s)
	return "", ""
}

const trickyExe = `C:\Users\O'Brien’s $env:TEMP ‘a‚b‛ $(calc)\Marstek Monitor\marstek-monitor.exe`

func TestPSQuoteAllSingleQuotes(t *testing.T) {
	for _, s := range []string{"", "plain", trickyExe, "''", "’", `a'; Remove-Item C:\ -Recurse; '`} {
		q := psQuote(s)
		val, rest := psLiteral(t, q)
		if val != s || rest != "" {
			t.Errorf("psQuote(%q) = %q: reads back as %q, rest %q", s, q, val, rest)
		}
	}
}

func TestFirewallCommand(t *testing.T) {
	cmd := firewallCommand(trickyExe)
	del := strings.Index(cmd, "Get-NetFirewallRule -DisplayName 'Marstek Monitor' -ErrorAction SilentlyContinue | Remove-NetFirewallRule")
	add := strings.Index(cmd, "New-NetFirewallRule -DisplayName 'Marstek Monitor' -Direction Inbound -Action Allow -Protocol UDP -Program ")
	if del < 0 || add < 0 || del > add {
		t.Fatalf("old rules must be removed before the new one is added:\n%s", cmd)
	}
	if !strings.HasPrefix(cmd, "$ErrorActionPreference = 'Stop'\n") {
		t.Errorf("a failure must stop the script (non-zero exit):\n%s", cmd)
	}
	// The elevated script loads NetSecurity from the system folder before any
	// of its cmdlets: auto-loading would search PSModulePath, which starts in
	// the user-writable Documents folder.
	if want := "$ErrorActionPreference = 'Stop'\n" + netSecurityImport + "\n"; !strings.HasPrefix(cmd, want) {
		t.Errorf("the script must start with %q:\n%s", want, cmd)
	}
	if netSecurityImport != `Import-Module "$PSHOME\Modules\NetSecurity\NetSecurity.psd1"` {
		t.Errorf("import line %q", netSecurityImport)
	}
	prog := cmd[add+len("New-NetFirewallRule -DisplayName 'Marstek Monitor' -Direction Inbound -Action Allow -Protocol UDP -Program "):]
	val, rest := psLiteral(t, prog)
	if val != trickyExe {
		t.Errorf("-Program reads as %q, want %q", val, trickyExe)
	}
	if rest != " -Profile Private | Out-Null\n" {
		t.Errorf("after the program: %q (the path leaked into the command)", rest)
	}
}

func TestEncodeCommand(t *testing.T) {
	script := "Write-Output 'Brien’s $x' \"\n"
	raw, err := base64.StdEncoding.DecodeString(encodeCommand(script))
	if err != nil || len(raw)%2 != 0 {
		t.Fatalf("not base64 of UTF-16: %v", err)
	}
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(raw[2*i:])
	}
	if got := string(utf16.Decode(u)); got != script {
		t.Errorf("decodes to %q, want %q", got, script)
	}
}

func TestParseFirewallProgramFilter(t *testing.T) {
	on := []byte(`{"Enabled":1}`)
	exe := `C:\Apps\Marstek\marstek-monitor.exe`
	moved := `D:\Old\marstek-monitor.exe`
	type rule struct {
		DisplayName string
		Enabled     int
		Program     string
	}
	for _, c := range []struct {
		name  string
		rules []rule
		want  bool
	}{
		{"this exe", []rule{{FirewallRuleName, 1, exe}}, false},
		{"this exe, other case", []rule{{FirewallRuleName, 1, strings.ToUpper(exe)}}, false},
		{"moved exe", []rule{{FirewallRuleName, 1, moved}}, true},
		{"any program", []rule{{FirewallRuleName, 1, "Any"}}, false},
		{"moved one + this one", []rule{{FirewallRuleName, 1, moved}, {FirewallRuleName, 1, exe}}, false},
		{"this exe, switched off", []rule{{FirewallRuleName, 2, exe}}, true},
		{"no program (lookup failed)", []rule{{FirewallRuleName, 1, ""}}, true},
	} {
		js, err := json.Marshal(c.rules)
		if err != nil {
			t.Fatal(err)
		}
		got, err := parseFirewallFor(on, js, exe)
		if err != nil || got != c.want {
			t.Errorf("%s: mayBlock = %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}

func TestLANAdaptersNeedAGateway(t *testing.T) {
	as := []adapter{
		{alias: "Ethernet", nets: []*net.IPNet{mustNet(t, "192.168.50.20/24")}, gateway: true},
		{alias: "vEthernet (WSL)", nets: []*net.IPNet{mustNet(t, "172.20.0.1/20")}},
	}
	nets, aliases := lanAdapters(as)
	if len(nets) != 1 || nets[0].String() != "192.168.50.20/24" || !aliases["Ethernet"] || aliases["vEthernet (WSL)"] {
		t.Errorf("nets %v, aliases %v", nets, aliases)
	}
	if nets, _ := lanAdapters(as[1:]); hasPrivateLAN(nets) {
		t.Error("only a virtual switch: want NoLAN")
	}
	// A VPN adapter on Public says nothing about the LAN's profile.
	js := []byte(`[{"InterfaceAlias":"Ethernet","NetworkCategory":1},{"InterfaceAlias":"VPN","NetworkCategory":0}]`)
	if pub, err := parseNetProfilesAmong(js, aliases); err != nil || pub {
		t.Errorf("LAN Private, VPN Public: public = %v, %v; want false", pub, err)
	}
	if pub, err := parseNetProfilesAmong(js, nil); err != nil || !pub {
		t.Errorf("all adapters: public = %v, %v; want true", pub, err)
	}
}

func TestParseFirewallNullProgram(t *testing.T) {
	got, err := parseFirewallFor([]byte(`{"Enabled":1}`), []byte(`{"DisplayName":"Marstek Monitor","Enabled":1,"Program":null}`), `C:\a\m.exe`)
	if err != nil || !got {
		t.Errorf("Program null: mayBlock = %v, %v; want true (hint stays)", got, err)
	}
}
