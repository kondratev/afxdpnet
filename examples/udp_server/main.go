//go:build linux && !tinygo

// udp_server is a simple UDP echo server using AF_XDP.
// Receives UDP packets on port 9000 and echoes back the payload.
//
// Usage:
//   sudo ./udp_server -i eth0
//
// Test with:
//   sudo ./udp_client -i eth0 -target 10.10.20.2 -port 9000 -msg "hello"
package main

import (
	"context"
	"flag"
	"net/netip"
	"os"
	"time"

	"github.com/kondratev/afxdpnet"
)

var DebugPrintf = afxdpnet.Printf
var DebugPrintln = afxdpnet.Println

func main() {
	var iface string

	flag.StringVar(&iface, "i", "eth0", "Interface to use")
	flag.Parse()

	DebugPrintf("[main] Creating AF_XDP stack on interface %q...\n", iface)

	stack, err := afxdpnet.New(iface, afxdpnet.Config{
		IP:     netip.MustParseAddr("10.10.20.2"),
		Subnet: netip.MustParsePrefix("10.10.20.0/24"),
	})
	if err != nil {
		DebugPrintf("[main] Error creating stack: %v\n", err)
		os.Exit(1)
	}
	defer stack.Close()

	DebugPrintf("[main] Stack created\n")

	// Create UDP listener using standard Go net API
	conn, err := stack.ListenUDP(9000)
	if err != nil {
		DebugPrintf("[udp] Error creating listener: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Start packet processing loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stack.Start(ctx)

	DebugPrintln("[udp] UDP echo server listening on port 9000")

	for {
		// Wait for UDP packet
		buf := make([]byte, 1500)
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if n == 0 {
			continue
		}

		DebugPrintf("[udp] Received %d bytes from %s: %s\n", n, remote.String(), string(buf[:n]))

		// Echo back
		_, err = conn.WriteTo(buf[:n], remote)
		if err != nil {
			DebugPrintf("[udp] Error sending echo: %v\n", err)
			continue
		}
		DebugPrintf("[udp] Echoed back to %s\n", remote.String())
	}
}
