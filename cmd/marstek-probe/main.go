// Command marstek-probe records raw Marstek local API replies to a JSONL file.
// It never calls Bat.GetStatus.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/econ11/marstek-monitor-code/core/source/lan"
)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func discover(ctx context.Context, c *lan.Client) ([]lan.DeviceInfo, error) {
	targets := lan.BroadcastTargets(lan.DefaultPort)
	devs, failed, err := c.DiscoverTargets(ctx, targets, 3*time.Second)
	for tgt, ferr := range failed {
		fmt.Printf("discovery: target %s failed: %v\n", tgt, ferr)
	}
	return devs, err
}

func realMain() error {
	ip := flag.String("ip", "", "station IP (skip discovery; required if several stations answer)")
	interval := flag.Duration("interval", 5*time.Second, "poll interval")
	gap := flag.Duration("gap", time.Second, "pause between calls within a tick")
	out := flag.String("out", "", "output JSONL file (default probe-<YYYYMMDD-HHMMSS>.jsonl)")
	flag.Parse()
	if *out == "" {
		*out = "probe-" + time.Now().Format("20060102-150405") + ".jsonl"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Bind the API port like the official clients; fall back to any port if busy.
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", lan.DefaultPort))
	if err != nil {
		fmt.Printf("WARNING: UDP port %d is busy (%v); using a random local port. If the station only replies to port %d, every call will time out.\n", lan.DefaultPort, err, lan.DefaultPort)
		if conn, err = net.ListenPacket("udp4", ":0"); err != nil {
			return err
		}
	}
	defer conn.Close()
	c := lan.NewClient(conn, 3*time.Second)

	var info lan.DeviceInfo
	var addr *net.UDPAddr
	if *ip != "" {
		a := net.ParseIP(*ip)
		if a == nil {
			return fmt.Errorf("bad -ip %q", *ip)
		}
		addr = &net.UDPAddr{IP: a, Port: lan.DefaultPort}
		// Best-effort identity; the probe still runs if the station does not answer.
		devs, _ := c.DiscoverMulti(ctx, []*net.UDPAddr{addr}, 3*time.Second)
		if len(devs) > 0 {
			info = devs[0]
		} else {
			info = lan.DeviceInfo{IP: *ip}
			fmt.Println("warning: station did not answer Marstek.GetDevice; continuing. Without its BLE MAC, automatic re-discovery after an IP change is not possible.")
		}
	} else {
		var devs []lan.DeviceInfo
		for i := 0; i < 3 && len(devs) == 0 && ctx.Err() == nil; i++ {
			if devs, err = discover(ctx, c); err != nil {
				return err
			}
		}
		for _, d := range devs {
			fmt.Printf("found: device=%q ver=%d ble_mac=%s ip=%s\n", d.Device, d.Ver, d.BLEMac, d.IP)
		}
		switch len(devs) {
		case 0:
			return fmt.Errorf("no station found; pass -ip")
		case 1:
		default:
			return fmt.Errorf("%d stations found; pass -ip to pick one", len(devs))
		}
		info = devs[0]
		sip := net.ParseIP(info.IP)
		if sip == nil {
			return fmt.Errorf("station reported bad ip %q", info.IP)
		}
		addr = &net.UDPAddr{IP: sip, Port: lan.DefaultPort}
	}

	f, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Println("writing", *out, "- Ctrl+C to stop")

	cfg := Config{
		Addr: addr, Info: info, Interval: *interval, Gap: *gap, Console: os.Stdout,
		Rediscover: func(ctx context.Context) (*net.UDPAddr, error) {
			devs, err := discover(ctx, c)
			if err != nil {
				return nil, err
			}
			for _, d := range devs {
				if d.BLEMac != "" && d.BLEMac == info.BLEMac {
					if nip := net.ParseIP(d.IP); nip != nil {
						return &net.UDPAddr{IP: nip, Port: lan.DefaultPort}, nil
					}
				}
			}
			return nil, fmt.Errorf("station %s not found", info.BLEMac)
		},
	}
	return Run(ctx, c, cfg, f)
}
