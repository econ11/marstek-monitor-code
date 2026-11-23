package app

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// Why a scan found nothing (flows 1.4 NONE_FOUND): the PowerShell output is
// parsed here, the commands run in netdiag_windows.go.

// FirewallRuleName is the display name of the inbound rule AllowFirewall
// adds and the diagnosis looks for.
const FirewallRuleName = "Marstek Monitor"

// Get-NetConnectionProfile | Select InterfaceAlias,NetworkCategory |
// ConvertTo-Json: NetworkCategory 0 = Public, 1 = Private, 2 = Domain.
// Windows PowerShell 5.1 prints the number, PowerShell 7 may print the name.
const categoryPublic = 0

// parseNetProfiles reports whether a connected network is Public (Windows
// then blocks the station's replies by default). Empty output means no
// connected network: not public.
func parseNetProfiles(js []byte) (public bool, err error) {
	return parseNetProfilesAmong(js, nil)
}

// parseNetProfilesAmong is parseNetProfiles over the interfaces named in
// aliases only (nil = all): a VPN adapter on Public says nothing about the
// LAN the station is on.
func parseNetProfilesAmong(js []byte, aliases map[string]bool) (public bool, err error) {
	var ps []struct {
		InterfaceAlias  string
		NetworkCategory json.RawMessage
	}
	if err := unmarshalList(js, &ps); err != nil {
		return false, fmt.Errorf("network profiles: %w", err)
	}
	for _, p := range ps {
		if aliases != nil && !aliases[p.InterfaceAlias] {
			continue
		}
		var n int
		var s string
		switch {
		case json.Unmarshal(p.NetworkCategory, &n) == nil && n == categoryPublic,
			json.Unmarshal(p.NetworkCategory, &s) == nil && strings.EqualFold(s, "Public"):
			return true, nil
		}
	}
	return false, nil
}

// parseFirewall reports whether the firewall may drop the station's replies:
// the Private profile is on (Get-NetFirewallProfile -Profile Private |
// Select Enabled) and no enabled rule named FirewallRuleName exists (empty
// rules output = none). The profile's output must not be empty.
func parseFirewall(profile, rules []byte) (mayBlock bool, err error) {
	return parseFirewallFor(profile, rules, "")
}

// parseFirewallFor is parseFirewall where a rule counts only if its program
// filter is exe (case-insensitive) or any program: a rule left behind for a
// moved exe does not let this one through. exe "" accepts every program.
// The rules output is DisplayName, Enabled and Program (rulesScript).
func parseFirewallFor(profile, rules []byte, exe string) (mayBlock bool, err error) {
	var ps []struct{ Enabled json.RawMessage }
	if err := unmarshalList(profile, &ps); err != nil {
		return false, fmt.Errorf("firewall profile: %w", err)
	}
	if len(ps) == 0 {
		return false, errors.New("firewall profile: no output")
	}
	on := false
	for _, p := range ps {
		// GpoBoolean: 0 False, 1 True, 2 NotConfigured (on by default).
		on = on || enabled(p.Enabled, 0)
	}
	if !on {
		return false, nil
	}
	var rs []struct {
		Enabled json.RawMessage
		Program string
	}
	if err := unmarshalList(rules, &rs); err != nil {
		return false, fmt.Errorf("firewall rules: %w", err)
	}
	for _, r := range rs {
		// The rule's Enabled enum: 1 True, 2 False.
		if (r.Enabled == nil || enabled(r.Enabled, 2)) && programMatches(r.Program, exe) {
			return false, nil
		}
	}
	return true, nil
}

// programMatches: a rule's program filter covers exe. Only an explicit
// "Any" covers every program; an empty or missing Program (the filter
// lookup failed) does not, so the hint stays. exe "" (parseFirewall: no exe
// to compare with) accepts every rule.
func programMatches(program, exe string) bool {
	if exe == "" || strings.EqualFold(program, "Any") {
		return true
	}
	if program == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(program), filepath.Clean(exe))
}

// enabled reads a PowerShell Enabled value: a JSON bool, "True"/"False", or
// a number where off is the number meaning False.
func enabled(raw json.RawMessage, off int) bool {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return !strings.EqualFold(s, "False")
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n != off
	}
	return true
}

// unmarshalList decodes ConvertTo-Json output into a slice: PowerShell
// prints one object without an array, and nothing at all for no objects.
func unmarshalList[T any](js []byte, out *[]T) error {
	js = bytes.TrimSpace(bytes.TrimPrefix(js, []byte("\xef\xbb\xbf")))
	if len(js) == 0 {
		*out = nil
		return nil
	}
	if js[0] == '[' {
		return json.Unmarshal(js, out)
	}
	var one T
	if err := json.Unmarshal(js, &one); err != nil {
		return err
	}
	*out = []T{one}
	return nil
}

// powerShellPath is Windows PowerShell 5.1 under the system directory
// (GetSystemDirectory): run by full path, never looked up on PATH or in the
// current directory, least of all elevated.
func powerShellPath(sysDir string) string {
	return filepath.Join(sysDir, `WindowsPowerShell\v1.0\powershell.exe`)
}

// psQuote is s as a PowerShell single-quoted literal. PowerShell takes the
// ASCII ' and the typographic ‘ ’ ‚ ‛ as single quotes; each is doubled, so
// nothing in s ends the literal ($ and ` stay literal inside it).
func psQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'', '‘', '’', '‚', '‛':
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}

// encodeCommand is script for powershell -EncodedCommand: base64 of its
// UTF-16LE, so no command-line quoting is involved.
func encodeCommand(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// netSecurityImport loads the firewall cmdlets' module from the Windows
// PowerShell folder ($PSHOME, under the system directory) by its path.
const netSecurityImport = `Import-Module "$PSHOME\Modules\NetSecurity\NetSecurity.psd1"`

// firewallCommand is the elevated script: drop every rule named
// FirewallRuleName (a stale, disabled or moved-exe one), then add the
// inbound UDP rule for exe on the Private profile. One UAC prompt does
// both; any failure exits non-zero. No -RemoteAddress LocalSubnet on
// purpose: a station typed by IP may be on another subnet (otherSubnet).
// NetSecurity is imported first by its file under $PSHOME, so its cmdlets
// never auto-load through PSModulePath (whose first folder is the user's
// writable Documents\WindowsPowerShell\Modules): a module planted there
// would run elevated.
func firewallCommand(exe string) string {
	name := psQuote(FirewallRuleName)
	return "$ErrorActionPreference = 'Stop'\n" +
		netSecurityImport + "\n" +
		"Get-NetFirewallRule -DisplayName " + name + " -ErrorAction SilentlyContinue | Remove-NetFirewallRule\n" +
		"New-NetFirewallRule -DisplayName " + name +
		" -Direction Inbound -Action Allow -Protocol UDP -Program " + psQuote(exe) + " -Profile Private | Out-Null\n"
}

// rulesScript lists the rules named FirewallRuleName with their program
// filter, for parseFirewallFor.
func rulesScript() string {
	return "Get-NetFirewallRule -DisplayName " + psQuote(FirewallRuleName) + " -ErrorAction SilentlyContinue | " +
		"ForEach-Object { [pscustomobject]@{ DisplayName = $_.DisplayName; Enabled = [int]$_.Enabled; " +
		"Program = ($_ | Get-NetFirewallApplicationFilter).Program } } | ConvertTo-Json"
}

// adapter is one network interface: its name (Get-NetConnectionProfile's
// InterfaceAlias), IPv4 networks, and whether it has a default gateway.
type adapter struct {
	alias   string
	nets    []*net.IPNet
	gateway bool
}

// lanAdapters keeps the adapters with a default gateway: the ones that lead
// to a router. Hyper-V, WSL and VMware switches carry private addresses too,
// but no gateway.
func lanAdapters(as []adapter) (nets []*net.IPNet, aliases map[string]bool) {
	aliases = map[string]bool{}
	for _, a := range as {
		if a.gateway {
			nets = append(nets, a.nets...)
			aliases[a.alias] = true
		}
	}
	return nets, aliases
}

// localNets are the IPv4 networks of the PC's interfaces that are up (not
// loopback).
func localNets() []*net.IPNet {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []*net.IPNet
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// hasPrivateLAN: one of nets has a private IPv4 (10/8, 172.16/12,
// 192.168/16), the kind a home router hands out. Without one the PC is not
// on the station's Wi-Fi (a mobile hotspot, a cable straight to the ISP).
func hasPrivateLAN(nets []*net.IPNet) bool {
	for _, n := range nets {
		if ip := n.IP.To4(); ip != nil && ip.IsPrivate() {
			return true
		}
	}
	return false
}

// inSubnets: ip is on one of nets.
func inSubnets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
