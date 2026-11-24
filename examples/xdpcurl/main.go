//go:build linux && !tinygo

// xdpcurl makes an HTTP request using AF_XDP sockets via the afxdpnet library.
//
// Usage:
//   sudo ./xdpcurl -i eth0 -target 10.10.20.2 -port 8080
//
// Requirements:
//   - Linux kernel with AF_XDP support (4.18+)
//   - go-afxdp library: go get github.com/atoonk/go-afxdp
//   - CAP_NET_RAW (root or sudo)
package main

import (
	"context"
	"flag"
	"fmt"
	"net/netip"
	"os"

	"github.com/soypat/lneto/http/httpraw"
	"github.com/kondratev/afxdpnet"
)

// Debug functions
var DebugPrintf = afxdpnet.Printf
var DebugPrintln = afxdpnet.Println

func main() {
	var (
		flagInterface string
		flagTarget    string
		flagPort      int
	)

	flag.StringVar(&flagInterface, "i", "eth0", "Interface to use")
	flag.StringVar(&flagTarget, "target", "10.10.20.2", "Target IP address")
	flag.IntVar(&flagPort, "port", 8080, "Target port")
	flag.Parse()

	DebugPrintln("[main] Starting xdpcurl...")
	DebugPrintf("[flags] interface=%q target=%q port=%d\n", flagInterface, flagTarget, flagPort)

	// Create the network stack
	DebugPrintln("[afxdpnet] Creating network stack...")
	stack, err := afxdpnet.New(flagInterface, afxdpnet.Config{
		IP:     netip.MustParseAddr("10.10.20.1"),
		Subnet: netip.MustParsePrefix("10.10.20.0/24"),
	})
	if err != nil {
		DebugPrintf("Error creating stack: %v\n", err)
		os.Exit(1)
	}
	defer stack.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stack.Start(ctx)

	// Create TCP connection
	DebugPrintf("[tcp] Connecting to %s:%d\n", flagTarget, flagPort)
	conn, err := stack.DialTCP(fmt.Sprintf("%s:%d", flagTarget, flagPort))
	if err != nil {
		DebugPrintf("Error dialing: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Create HTTP request
	DebugPrintln("[http] Sending HTTP request...")
	var hdr httpraw.HeaderV1
	hdr.SetMethod("GET")
	hdr.SetRequestTarget("/")
	hdr.SetProtocol("HTTP/1.1")
	hdr.Set("Host", flagTarget)
	hdr.Set("User-Agent", "xdpcurl")
	hdr.Set("Connection", "close")
	req, err := hdr.AppendRequest(nil)
	if err != nil {
		DebugPrintf("Error creating HTTP request: %v\n", err)
		os.Exit(1)
	}

	// Send HTTP request
	_, err = conn.Write(req)
	if err != nil {
		DebugPrintf("Error sending HTTP request: %v\n", err)
		os.Exit(1)
	}
	DebugPrintln("[http] Request sent")

	// Receive HTTP response
	DebugPrintln("[http] Waiting for response...")
	rxbuf := make([]byte, 4096)
	var page []byte
	for {
		n, err := conn.Read(rxbuf)
		if n > 0 {
			page = append(page, rxbuf[:n]...)
			DebugPrintf("[http] Received %d bytes (total %d)\n", n, len(page))
		}
		if err != nil {
			break
		}
		if len(page) >= 7 && string(page[len(page)-7:]) == "</html>" {
			DebugPrintln("[http] Complete HTML received")
			break
		}
	}

	if len(page) == 0 {
		DebugPrintln("[http] No HTTP response received")
	} else {
		DebugPrintf("[http] Total response: %d bytes\n", len(page))
		DebugPrintln("[http] Response:")
		DebugPrintln(string(page))
	}

	DebugPrintln("[main] Done!")
}
