//go:build linux && !tinygo

// xdpsrv_lib is a simple HTTP server using the afxdpnet library.
// Demonstrates how to use the library to create a net-compatible stack.
//
// Usage:
//   sudo ./xdpsrv_lib -i eth0
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

// Debug functions from afxdpnet
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

	// Create listener using standard Go net API
	listener, err := stack.ListenTCP(8080)
	if err != nil {
		DebugPrintf("[tcp] Error creating listener: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	// Start packet processing loop AFTER listener is registered
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stack.Start(ctx)

	DebugPrintln("[tcp] Server is ready and waiting for connections...")

	// Accept connections using standard Go net API
	for {
		conn, err := listener.Accept()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		addr := conn.RemoteAddr().String()
		DebugPrintf("[tcp] Connection accepted from %s\n", addr)

		// Handle the connection in a separate goroutine
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Read HTTP request
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		DebugPrintln("[conn] Read error")
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
		DebugPrintln("[conn] Error writing response")
		return
	}

	DebugPrintln("[conn] Response sent")
}
