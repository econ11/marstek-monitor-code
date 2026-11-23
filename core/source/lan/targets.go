package lan

import "net"

// DirectedBroadcast is the broadcast address of the IPv4 network n
// (192.168.50.5/24 -> 192.168.50.255); nil if n is not IPv4.
func DirectedBroadcast(n *net.IPNet) net.IP {
	ip, m := n.IP.To4(), n.Mask
	if len(m) == net.IPv6len {
		m = m[12:]
	}
	if ip == nil || len(m) != net.IPv4len {
		return nil
	}
	b := make(net.IP, net.IPv4len)
	for i := range b {
		b[i] = ip[i] | ^m[i]
	}
	return b
}

// BroadcastTargets returns the discovery targets on port: 255.255.255.255
// (which Windows sends out of one interface only) plus the directed broadcast
// of every up, non-loopback IPv4 interface.
func BroadcastTargets(port int) []*net.UDPAddr {
	t := []*net.UDPAddr{{IP: net.IPv4bcast, Port: port}}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if b := DirectedBroadcast(ipn); b != nil {
					t = append(t, &net.UDPAddr{IP: b, Port: port})
				}
			}
		}
	}
	return t
}
