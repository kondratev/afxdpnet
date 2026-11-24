//go:build linux && !tinygo

// httpserver_fasthttp is an HTTP/1.1 server using fasthttp library.
// Demonstrates that our TCP implementation is compatible with popular HTTP libraries.
//
// Usage:
//   sudo ./httpserver_fasthttp -i eth0
//   curl http://10.10.20.2:8080/
//   curl http://10.10.20.2:8080/hello
package main

import (
	"bytes"
	"context"
	"flag"
	"net/netip"
	"os"
	"time"

	"github.com/soypat/lneto/http/httpraw"
	"github.com/valyala/fasthttp"
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

	// Create TCP listener using standard Go net API
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

	DebugPrintln("[http] HTTP server is ready on port 8080")
	DebugPrintln("[http] Try: curl http://10.10.20.2:8080/")
	DebugPrintln("[http] Try: curl http://10.10.20.2:8080/hello")

	// Accept loop using standard Go net API
	for {
		conn, err := listener.Accept()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		addr := conn.RemoteAddr().String()
		DebugPrintf("[tcp] Connection accepted from %s\n", addr)

		// Handle HTTP request
		go handleHTTPRequest(conn)
	}
}

func handleHTTPRequest(conn any) {
	defer conn.(interface{ Close() error }).Close()

	// Read HTTP request using lneto's HTTP parser
	buf := make([]byte, 4096)
	n, err := conn.(interface{ Read([]byte) (int, error) }).Read(buf)
	if err != nil || n == 0 {
		return
	}

	// Parse HTTP request using lneto's httpraw parser
	var hdr httpraw.HeaderV1
	err = hdr.ParseBytes(false, buf[:n])
	if err != nil {
		DebugPrintf("[http] Error parsing request: %v\n", err)
		return
	}

	DebugPrintf("[http] Request: %s %s\n",
		hdr.Method(), hdr.RequestTarget())

	// Build response using fasthttp's response writer
	var resp fasthttp.Response
	switch string(hdr.RequestTarget()) {
	case "/":
		resp.SetBodyString("<html><body><h1>Hello from AF_XDP + fasthttp!</h1></body></html>")
	case "/hello":
		resp.SetBodyString("Hello from AF_XDP + fasthttp!")
	case "/time":
		resp.SetBodyString(time.Now().Format(time.RFC1123))
	default:
		resp.SetStatusCode(fasthttp.StatusNotFound)
		resp.SetBodyString("404 Not Found")
	}

	// Write full HTTP response
	var respBuf bytes.Buffer
	resp.WriteTo(&respBuf)
	_, err = conn.(interface{ Write([]byte) (int, error) }).Write(respBuf.Bytes())
	if err != nil {
		DebugPrintf("[http] Error writing response: %v\n", err)
		return
	}

	DebugPrintln("[http] Response sent")
}
