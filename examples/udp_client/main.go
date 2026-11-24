//go:build linux && !tinygo

// udp_client sends a UDP packet to a target and waits for a response.
// Demonstrates AF_XDP UDP communication.
//
// Usage:
//   sudo ./udp_client -i eth0 -target 10.10.20.2 -port 9000 -msg "hello"
package main

import (
	"context"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"time"

	"github.com/kondratev/afxdpnet"
)

var DebugPrintf = afxdpnet.Printf
var DebugPrintln = afxdpnet.Println

func main() {
	var iface, target string
	var port int
	var msg string

	flag.StringVar(&iface, "i", "eth0", "Interface to use")
	flag.StringVar(&target, "target", "10.10.20.2", "Target IP address")
	flag.IntVar(&port, "port", 9000, "Target port")
	flag.StringVar(&msg, "msg", "Hello from AF_XDP!", "Message to send")
	flag.Parse()

	DebugPrintf("[main] Creating AF_XDP stack on interface %q...\n", iface)

	stack, err := afxdpnet.New(iface, afxdpnet.Config{
		IP:     netip.MustParseAddr("10.10.20.1"),
		Subnet: netip.MustParsePrefix("10.10.20.0/24"),
	})
	if err != nil {
		DebugPrintf("[main] Error creating stack: %v\n", err)
		os.Exit(1)
	}
	defer stack.Close()

	DebugPrintf("[main] Stack created\n")

	// Resolve target MAC via ARP first
	DebugPrintf("[arp] Resolving MAC for %s...\n", target)
	remote := netip.AddrPortFrom(netip.MustParseAddr(target), uint16(port))
	if err := stack.ResolveHardwareAddress6(remote.Addr()); err != nil {
		DebugPrintf("[arp] Error resolving: %v\n", err)
		os.Exit(1)
	}

	// Start packet processing loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stack.Start(ctx)

	// Wait for ARP resolution
	deadline := time.Now().Add(5 * time.Second)
	hwResolved := false
	for time.Now().Before(deadline) {
		hw, err := stack.ResultResolveHardwareAddress6(remote.Addr())
		if err == nil {
			DebugPrintf("[arp] Resolved: %02x:%02x:%02x:%02x:%02x:%02x\n",
				hw[0], hw[1], hw[2], hw[3], hw[4], hw[5])
			hwResolved = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !hwResolved {
		DebugPrintf("[arp] ARP resolution failed\n")
		os.Exit(1)
	}

	// Create UDP connection using standard Go net API
	DebugPrintf("[udp] Creating connection to %s\n", remote.String())
	conn, err := stack.DialUDP(fmt.Sprintf("%s:%d", target, port))
	if err != nil {
		DebugPrintf("[udp] Error creating connection: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Send UDP packet
	DebugPrintf("[udp] Sending: %s\n", msg)
	_, err = conn.Write([]byte(msg))
	if err != nil {
		DebugPrintf("[udp] Error sending: %v\n", err)
		os.Exit(1)
	}

	// Wait for response
	DebugPrintln("[udp] Waiting for response...")
	buf := make([]byte, 1500)
	deadline = time.Now().Add(5 * time.Second)
	received := false
	for time.Now().Before(deadline) {
		n, err := conn.Read(buf)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if n == 0 {
			continue
		}
		DebugPrintf("[udp] Received: %s\n", string(buf[:n]))
		received = true
		break
	}
	if !received {
		DebugPrintln("[udp] Timeout waiting for response")
		os.Exit(1)
	}

	DebugPrintln("[main] Done!")
}
