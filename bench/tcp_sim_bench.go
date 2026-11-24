//go:build linux && !tinygo && bench

// tcp_sim_bench benchmarks TCP throughput using a simulated network
// with two TCP Handlers directly (like lneto's own tests).
// This isolates the TCP stack from the network adapter to measure
// raw TCP performance.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/soypat/lneto/ethernet"
	"github.com/soypat/lneto/tcp"
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

	flag.IntVar(&conns, "conns", 10, "Number of concurrent connections")
	flag.IntVar(&duration, "duration", 5, "Test duration in seconds")
	flag.Parse()

	fmt.Printf("TCP stack benchmark (simulated network): %d connections, %d seconds\n", conns, duration)
	fmt.Println("=============================================================")

	// Run benchmark
	startTime := time.Now()
	for i := 0; i < conns; i++ {
		go runConnection(i, startTime, time.Duration(duration)*time.Second)
	}

	// Wait for duration
	time.Sleep(time.Duration(duration)*time.Second + time.Second)

	elapsed := time.Since(startTime)
	rps := float64(requestCount) / elapsed.Seconds()

	fmt.Printf("\nResults:\n")
	fmt.Printf("Total requests: %d\n", requestCount)
	fmt.Printf("Duration: %.2fs\n", elapsed.Seconds())
	fmt.Printf("Throughput: %.2f req/s\n", rps)
}

func runConnection(id int, startTime time.Time, duration time.Duration) {
	rng := rand.New(rand.NewSource(int64(id) + 1))

	for {
		elapsed := time.Since(startTime)
		if elapsed >= duration {
			return
		}

		// Create TCP handlers (like lneto tests)
		client := newHandler(rng)
		server := newHandler(rng)

		// Setup client/server
		port := uint16(rng.Uint32())
		err := server.OpenListen(port, 0)
		if err != nil {
			continue
		}

		err = client.OpenActive(uint16(rng.Uint32()), port, 0)
		if err != nil {
			continue
		}

		// Establish connection
		packetBuf := make([]byte, mtu)
		err = establish(client, server, packetBuf)
		if err != nil {
			continue
		}

		// Send request
		req := "GET / HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"
		_, err = client.Write([]byte(req))
		if err != nil {
			continue
		}

		// Send data
		n, err := client.Send(packetBuf)
		if err != nil || n == 0 {
			continue
		}

		// Server receives
		err = server.Recv(packetBuf[:n])
		if err != nil {
			continue
		}

		// Server reads
		buf := make([]byte, 4096)
		_, err = server.Read(buf)
		if err != nil {
			continue
		}

		// Server sends response
		resp := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nOK"
		_, err = server.Write([]byte(resp))
		if err != nil {
			continue
		}

		n, err = server.Send(packetBuf)
		if err != nil || n == 0 {
			continue
		}

		// Client receives response
		err = client.Recv(packetBuf[:n])
		if err != nil {
			continue
		}

		_, err = client.Read(buf)
		if err != nil {
			continue
		}

		// Close connections
		client.Close()
		server.Close()

		requestCount++
	}
}

func newHandler(rng *rand.Rand) *tcp.Handler {
	h := new(tcp.Handler)
	err := h.SetBuffers(make([]byte, mtu), make([]byte, mtu), 4)
	if err != nil {
		fmt.Printf("Error setting buffers: %v\n", err)
		os.Exit(1)
	}
	return h
}

func establish(client, server *tcp.Handler, packetBuf []byte) error {
	// Client sends SYN
	n, err := client.Send(packetBuf)
	if err != nil || n < 20 {
		return err
	}
	err = server.Recv(packetBuf[:n])
	if err != nil {
		return err
	}

	// Server sends SYN-ACK
	n, err = server.Send(packetBuf)
	if err != nil || n < 20 {
		return err
	}
	err = client.Recv(packetBuf[:n])
	if err != nil {
		return err
	}

	// Client sends ACK
	n, err = client.Send(packetBuf)
	if err != nil || n < 20 {
		return err
	}
	err = server.Recv(packetBuf[:n])
	if err != nil {
		return err
	}

	return nil
}