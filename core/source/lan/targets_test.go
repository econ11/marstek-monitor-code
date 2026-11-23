package lan

import (
	"net"
	"testing"
)

func TestDirectedBroadcast(t *testing.T) {
	for cidr, want := range map[string]string{
		"192.168.50.5/24": "192.168.50.255",
		"10.1.2.3/8":      "10.255.255.255",
		"172.16.5.9/20":   "172.16.15.255",
		"192.168.1.7/32":  "192.168.1.7",
	} {
		ip, n, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		n.IP = ip // interface addresses keep the host part
		if got := DirectedBroadcast(n); got.String() != want {
			t.Errorf("DirectedBroadcast(%s) = %v, want %s", cidr, got, want)
		}
	}
	// A 16-byte mask, as some platforms report for IPv4 addresses.
	n := &net.IPNet{IP: net.ParseIP("192.168.50.5"), Mask: net.CIDRMask(120, 128)}
	if got := DirectedBroadcast(n); got.String() != "192.168.50.255" {
		t.Errorf("16-byte mask: %v", got)
	}
	_, v6, _ := net.ParseCIDR("fe80::1/64")
	if got := DirectedBroadcast(v6); got != nil {
		t.Errorf("IPv6: %v, want nil", got)
	}
}

func TestBroadcastTargets(t *testing.T) {
	ts := BroadcastTargets(30000)
	if len(ts) == 0 || !ts[0].IP.Equal(net.IPv4bcast) {
		t.Fatalf("targets = %v, want 255.255.255.255 first", ts)
	}
	for _, a := range ts {
		if a.Port != 30000 || a.IP.To4() == nil || a.IP.IsLoopback() {
			t.Errorf("bad target %v", a)
		}
	}
}
