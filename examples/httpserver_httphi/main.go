//go:build linux && !tinygo

// httpserver_httphi is a proper HTTP/1.1 server using the afxdpnet library and httphi router.
// Demonstrates that our TCP implementation is compatible with standard HTTP/1.1 protocols.
//
// Usage:
//   sudo ./httpserver_httphi -i eth0
//   curl http://10.10.20.2:8080/
//   curl http://10.10.20.2:8080/hello
package main

import (
	"context"
	"flag"
	"net/netip"
	"os"
	"time"

	"github.com/soypat/lneto/http/httphi"
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

	// Setup HTTP/1.1 router with httphi
	var mux httphi.MuxSlice

	mux.Handle("GET /", func(ex *httphi.Exchange) {
		ex.RespondString(httphi.StatusOK, "text/html", "<html><body><h1>Hello from AF_XDP!</h1><p>HTTP/1.1 works correctly</p></body></html>")
	})

	mux.Handle("GET /hello", func(ex *httphi.Exchange) {
		ex.RespondString(httphi.StatusOK, "text/plain", "Hello from AF_XDP HTTP/1.1 server!")
	})

	mux.Handle("GET /time", func(ex *httphi.Exchange) {
		ex.RespondString(httphi.StatusOK, "text/plain", time.Now().Format(time.RFC1123))
	})

	mux.Handle("POST /echo", func(ex *httphi.Exchange) {
		body := make([]byte, 4096)
		n, err := ex.ReadBody(body)
		if err != nil {
			ex.RespondString(httphi.StatusBadRequest, "text/plain", "error reading body")
			return
		}
		ex.RespondString(httphi.StatusOK, "text/plain", string(body[:n]))
	})

	// Configure router
	// FixedNumGoroutines = -1 means unbounded (goroutine per connection)
	cfg := httphi.DefaultRouterConfig(-1, 2048, mux.MaxPathValues())
	var router httphi.Router
	if err := router.Configure(&mux, cfg); err != nil {
		DebugPrintf("[http] Error configuring router: %v\n", err)
		os.Exit(1)
	}
	defer router.Shutdown()

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

	DebugPrintln("[http] HTTP/1.1 server is ready on port 8080")
	DebugPrintln("[http] Try: curl http://10.10.20.2:8080/")
	DebugPrintln("[http] Try: curl http://10.10.20.2:8080/hello")
	DebugPrintln("[http] Try: curl http://10.10.20.2:8080/time")

	// Accept loop using standard Go net API
	for {
		conn, err := listener.Accept()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		addr := conn.RemoteAddr().String()
		DebugPrintf("[tcp] Connection accepted from %s\n", addr)

		// Handle HTTP request using httphi router
		// Router takes io.ReadWriteCloser, which net.Conn implements
		if err := router.Handle(conn); err != nil {
			DebugPrintf("[http] Error handling request from %s: %v\n", addr, err)
			conn.Close()
		} else {
			DebugPrintf("[http] Request from %s handled successfully\n", addr)
		}
	}
}
