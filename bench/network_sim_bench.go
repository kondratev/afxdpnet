//go:build linux && !tinygo && bench

// network_sim_bench benchmarks HTTP throughput using two StackAsync instances
// communicating through a simulated network. This measures the full TCP/IP stack
// performance including Ethernet framing, IP routing, TCP handshake, and TCP
// state management — but without actual AF_XDP hardware.
//
// The test drives the simulated network by repeatedly calling EgressEthernet on
// each stack and feeding the output to the other stack's IngressEthernet, until
// no more packets are produced (quiescence).
//
// This is analogous to the "ping/pong" test in the lneto test suite, but using
// real TCP/IP networking with a listener and connection pool instead of UDP.
//
// Usage: go run network_sim_bench.go -conns 10 -duration 5

package main

import (
	"flag"
	"fmt"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/soypat/lneto"
	"github.com/soypat/lneto/ethernet"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/x/xnet"
)

const (
	mtu = ethernet.MaxMTU
)

var (
	requestCount int64
)

func main() {
	var conns int
	var duration int
	var queueSize int
	var bufSize int
	var poolSize int

	flag.IntVar(&conns, "conns", 10, "Number of concurrent connections")
	flag.IntVar(&duration, "duration", 5, "Test duration in seconds")
	flag.IntVar(&queueSize, "queue", 150, "TCP listener queue size")
	flag.IntVar(&bufSize, "bufsize", 6000, "TCP buffer size per connection")
	flag.IntVar(&poolSize, "poolsize", 2000, "TCP pool size")
	flag.Parse()

	fmt.Printf("Simulated network HTTP benchmark: %d connections, %d seconds\n", conns, duration)
	fmt.Printf("Config: pool=%d, queue=%d, bufsize=%d\n", poolSize, queueSize, bufSize)
	fmt.Println("=============================================================")

	// Run the benchmark
	startTime := time.Now()
	runBenchmark(conns, time.Duration(duration)*time.Second, queueSize, bufSize, uint16(poolSize))
	elapsed := time.Since(startTime)

	fmt.Printf("\nResults:\n")
	fmt.Printf("Total requests: %d\n", requestCount)
	fmt.Printf("Duration: %.2fs\n", elapsed.Seconds())
	if elapsed.Seconds() > 0 {
		fmt.Printf("Throughput: %.2f req/s\n", float64(requestCount)/elapsed.Seconds())
	}
}

func runBenchmark(conns int, duration time.Duration, queueSize, bufSize int, poolSize uint16) {
	startTime := time.Now()

	// Create server stack
	sv := new(xnet.StackAsync)
	err := sv.Reset(xnet.StackConfig{
		Hostname:          "server",
		RandSeed:          1,
		StaticAddress4:    [4]byte{10, 10, 20, 2},
		MaxActiveTCPPorts: 2000,
		HardwareAddress:   [6]byte{0x52, 0x54, 0x00, 0x44, 0x55, 0x66},
		MTU:               mtu,
		PassivePeers:      20,
	})
	if err != nil {
		fmt.Printf("Error resetting server: %v\n", err)
		return
	}

	// Create client stack
	cl := new(xnet.StackAsync)
	err = cl.Reset(xnet.StackConfig{
		Hostname:          "client",
		RandSeed:          2,
		StaticAddress4:    [4]byte{10, 10, 20, 1},
		MaxActiveTCPPorts: 2000,
		HardwareAddress:   [6]byte{0x52, 0x54, 0x00, 0x11, 0x22, 0x33},
		MTU:               mtu,
		PassivePeers:      20,
	})
	if err != nil {
		fmt.Printf("Error resetting client: %v\n", err)
		return
	}

	// Set gateway MAC addresses (for ARP resolution)
	sv.SetGatewayHardwareAddr(cl.HardwareAddr())
	cl.SetGatewayHardwareAddr(sv.HardwareAddr())

	// Create TCP pool for listener
	pool, err := sv.NewTCPPool(xnet.TCPPoolConfig{
		PoolSize:           poolSize,
		QueueSize:          queueSize,
		TxBufSize:          bufSize,
		RxBufSize:          bufSize,
		EstablishedTimeout: 30 * time.Second,
		ClosingTimeout:     500 * time.Millisecond,
		NewBackoff: func() lneto.BackoffStrategy {
			return func(n uint) time.Duration { return 0 }
		},
	})
	if err != nil {
		fmt.Printf("Error creating pool: %v\n", err)
		return
	}

	// Create listener
	var listener tcp.Listener
	err = listener.Reset(8080, pool)
	if err != nil {
		fmt.Printf("Error resetting listener: %v\n", err)
		return
	}
	err = sv.RegisterListenerTCP(&listener)
	if err != nil {
		fmt.Printf("Error registering listener: %v\n", err)
		return
	}

	// Buffer for network simulation
	buf := make([]byte, mtu+ethernet.MaxOverheadSize)

	// Helper to drive network simulation until quiescence
	ensureQuiesce := func() {
		for i := 0; i < 10; i++ {
			sent := false
			// Client -> Server
			n, err := cl.EgressEthernet(buf)
			if err == nil && n > 0 {
				sv.IngressEthernet(buf[:n])
				sent = true
			}
			// Server -> Client
			n, err = sv.EgressEthernet(buf)
			if err == nil && n > 0 {
				cl.IngressEthernet(buf[:n])
				sent = true
			}
			if !sent {
				break
			}
		}
	}

	// Helper to complete TCP handshake
	handshake := func(clConn *tcp.Conn) (*tcp.Conn, error) {
		ensureQuiesce()

		if listener.NumberOfReadyToAccept() != 1 {
			return nil, fmt.Errorf("expected 1 ready to accept, got %d (client state: %s)", listener.NumberOfReadyToAccept(), clConn.State())
		}
		pinned, err := listener.TryAccept()
		if err != nil {
			return nil, err
		}

		if clConn.State() != tcp.StateEstablished {
			return nil, fmt.Errorf("client state: %s", clConn.State())
		}

		return pinned.Conn(), nil
	}

	// Create client connection
	var clientConn tcp.Conn
	err = clientConn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, mtu*4),
		TxBuf:             make([]byte, mtu*4),
		TxPacketQueueSize: 4,
		RWBackoff:         func(n uint) time.Duration { return 0 },
	})
	if err != nil {
		fmt.Printf("Error configuring client conn: %v\n", err)
		return
	}

	// Server address
	svAddr := netip.AddrPortFrom(netip.AddrFrom4([4]byte{10, 10, 20, 2}), 8080)

	// Run HTTP request/response loop
	for i := 0; ; i++ {
		elapsed := time.Since(startTime)
		if elapsed >= duration {
			break
		}

		// Client dials server (must specify local port)
		localPort := uint16(10000 + i%1000)
		err = cl.DialTCP(&clientConn, localPort, svAddr)
		if err != nil {
			continue
		}

		// Complete handshake
		svConn, err := handshake(&clientConn)
		if err != nil {
			continue
		}

		// Send HTTP request
		req := "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"
		_, err = clientConn.Write([]byte(req))
		if err != nil {
			continue
		}
		ensureQuiesce()

		// Server reads request and sends response
		svBuf := make([]byte, 4096)
		_, err = svConn.Read(svBuf)
		if err == nil {
			resp := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nOK"
			svConn.Write([]byte(resp))
		}
		ensureQuiesce()

		// Client reads response
		clBuf := make([]byte, 4096)
		n, err := clientConn.Read(clBuf)
		if err == nil && n > 0 {
			atomic.AddInt64(&requestCount, 1)
		}

		// Close connections
		clientConn.Close()
		ensureQuiesce()
		svConn.Close()
		ensureQuiesce()
	}
}
