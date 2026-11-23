package app

import (
	"errors"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// On macOS there is no network profile (Public / Private) and the
// Application Firewall lets the replies to the app's own requests through.
// What does stop a LAN app is the Local Network privacy permission (macOS
// 15+): without it every packet to a LAN address fails with "no route to
// host". So the Windows checks map as:
//
//   - PublicNetwork → always false;
//   - FirewallMayBlock → "Local Network access is denied" (probe below);
//   - AllowFirewall → opens System Settings → Privacy & Security → Local
//     Network (the app cannot grant it itself).

// systemPublicNetwork: macOS has no network categories.
func systemPublicNetwork(map[string]bool) (bool, error) { return false, nil }

// systemFirewallMayBlock reports whether macOS denies the app Local Network
// access: one empty UDP datagram to the default gateway's discard port (9)
// fails with EHOSTUNREACH then. Without a gateway nothing is known.
func systemFirewallMayBlock() (bool, error) {
	gw, _, err := defaultRoute()
	if err != nil || gw == nil {
		return false, err
	}
	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: gw, Port: 9})
	if err != nil {
		return errors.Is(err, syscall.EHOSTUNREACH), nil
	}
	defer c.Close()
	_ = c.SetWriteDeadline(time.Now().Add(time.Second))
	_, err = c.Write([]byte{})
	return errors.Is(err, syscall.EHOSTUNREACH), nil
}

// systemAdapters lists the interfaces that are up, with their IPv4 networks;
// the one the default route leaves through has the gateway.
func systemAdapters() ([]adapter, error) {
	_, gwIf, _ := defaultRoute() // no default route: no adapter has a gateway
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []adapter
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		ad := adapter{alias: ifc.Name, gateway: ifc.Name == gwIf}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				ad.nets = append(ad.nets, n)
			}
		}
		out = append(out, ad)
	}
	return out, nil
}

// defaultRoute is the IPv4 default gateway and its interface ("en0"), from
// `route -n get default`.
func defaultRoute() (gw net.IP, ifName string, err error) {
	out, err := exec.Command("/sbin/route", "-n", "get", "default").Output()
	if err != nil {
		return nil, "", nil // "not in table": no default route
	}
	gw, ifName = parseRouteGet(string(out))
	return gw, ifName, nil
}

// parseRouteGet reads "gateway:" and "interface:" from `route -n get`.
func parseRouteGet(out string) (gw net.IP, ifName string) {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "gateway":
			gw = net.ParseIP(strings.TrimSpace(v)).To4()
		case "interface":
			ifName = strings.TrimSpace(v)
		}
	}
	return gw, ifName
}

// localNetworkSettings is System Settings → Privacy & Security → Local
// Network.
const localNetworkSettings = "x-apple.systempreferences:com.apple.preference.security?Privacy_LocalNetwork"

// systemAllowFirewall opens the Local Network pane; the user switches the
// app on there. running is closed at once: no prompt stays open.
func systemAllowFirewall(uintptr) (<-chan struct{}, error) {
	done := make(chan struct{})
	close(done)
	return done, exec.Command("open", localNetworkSettings).Start()
}
