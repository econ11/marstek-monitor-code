package app

import "testing"

func TestParseRouteGet(t *testing.T) {
	out := `   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>
`
	gw, ifName := parseRouteGet(out)
	if gw.String() != "192.168.1.1" || ifName != "en0" {
		t.Fatalf("gw %v if %q", gw, ifName)
	}
	if gw, ifName := parseRouteGet("route: writing to routing socket: not in table\n"); gw != nil || ifName != "" {
		t.Fatalf("no route: gw %v if %q", gw, ifName)
	}
}
