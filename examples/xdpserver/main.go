//go:build linux && !tinygo

// xdpserver is a simple HTTP server using AF_XDP sockets via the afxdpnet library.
//
// Usage:
//   sudo ./xdpserver -i eth0
//
// Requirements:
//   - Linux kernel with AF_XDP support (4.18+)
//   - go-afxdp library: go get github.com/atoonk/go-afxdp
//   - CAP_NET_RAW (root or sudo)
//   - Checksum offloading disabled: sudo ethtool -K eth0 tx off
package main

import (
	"context"
	"flag"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/kondratev/afxdpnet"
)

const listenPort = 8080

// Debug functions
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

	DebugPrintln("[main] Stack created")

	// Create listener using standard Go net API
	listener, err := stack.ListenTCP(listenPort)
	if err != nil {
		DebugPrintf("[main] Error creating listener: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	DebugPrintf("[tcp] Listening on port %d...\n", listenPort)
	DebugPrintln("[tcp] Waiting for connections...")

	// Start packet processing loop AFTER listener is registered
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stack.Start(ctx)

	DebugPrintln("[tcp] Server is ready and waiting for connections")

	for {
		conn, err := listener.Accept()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		addrPort := conn.RemoteAddr().String()
		DebugPrintf("[tcp] Connection accepted from %s\n", addrPort)

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Read HTTP request
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		DebugPrintf("[conn] Read error: %v\n", err)
		return
	}

	DebugPrintf("[conn] Received %d bytes from %s\n", n, conn.RemoteAddr().String())

	// Build HTTP response
	resp := "HTTP/1.1 200 OK\r\n"
	resp += "Content-Type: text/html\r\n"
	resp += "Content-Length: 52\r\n"
	resp += "Connection: close\r\n"
	resp += "\r\n"
	resp += "<html><body><h1>Hello from xdpserver!</h1></body></html>"

	_, err = conn.Write([]byte(resp))
	if err != nil {
		DebugPrintf("[conn] Error writing response: %v\n", err)
		return
	}

	DebugPrintln("[conn] Response sent")
}
