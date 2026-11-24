# afxdpnet - AF_XDP Network Stack with lneto

A high-performance network stack using AF_XDP sockets for packet I/O, integrated with the lneto TCP/IP stack. Provides a simple API for creating TCP/UDP listeners and connections.

## Architecture

```
Application
    ↓
afxdpnet (library) - Network stack management
    ↓
lneto/x/xnet - TCP/IP stack with passive peer learning
    ↓
afxdp (adapter) - AF_XDP low-level operations
    ↓
go-afxdp - AF_XDP socket management (Fleet API)
    ↓
Linux AF_XDP
```

## Requirements

- Linux kernel 4.18+ with AF_XDP support
- go-afxdp library: `go get github.com/atoonk/go-afxdp`
- CAP_NET_RAW and CAP_SYS_ADMIN (run as root or with sudo)
- Checksum offloading disabled on the interface: `sudo ethtool -K eth0 tx off rx off`

## Quick Start

### Creating a network stack

```go
import (
    "context"
    "net/netip"
    "github.com/kondratev/afxdpnet"
)

stack, err := afxdpnet.New("eth0", afxdpnet.Config{
    IP:     netip.MustParseAddr("10.10.20.1"),
    Subnet: netip.MustParsePrefix("10.10.20.0/24"),
})
if err != nil {
    log.Fatal(err)
}
defer stack.Close()

// Start packet processing loop
ctx := context.Background()
stack.Start(ctx)
```

### TCP Server

```go
// Create listener using standard Go net API
listener, err := stack.ListenTCP(8080)
if err != nil {
    log.Fatal(err)
}
defer listener.Close()

// Accept connections
for {
    conn, err := listener.Accept()
    if err != nil {
        time.Sleep(10 * time.Millisecond)
        continue
    }
    go handleConnection(conn)
}
```

### TCP Client

```go
// Dial using standard Go API
conn, err := stack.DialTCP("10.10.20.2:8080")
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// Use conn as net.Conn
conn.Write([]byte("Hello!"))
```

### UDP Listener

```go
// Create UDP listener using standard Go net API
conn, err := stack.ListenUDP(9000)
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// Read packets
buf := make([]byte, 1500)
n, remote, err := conn.ReadFrom(buf)
```

### UDP Client

```go
// Dial UDP using standard Go API
conn, err := stack.DialUDP("10.10.20.2:9000")
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// Send packets
conn.Write([]byte("Hello from AF_XDP!"))
```

## Examples

### Library-based examples (recommended)

| Example | Description |
|---------|-------------|
| `examples/xdpserver` | Simple HTTP server using lneto TCP listener |
| `examples/xdpcurl` | HTTP client making requests with lneto TCP dial |
| `examples/httpserver_fasthttp` | HTTP server using fasthttp with persistent connections |
| `examples/httpserver_httphi` | HTTP server using httphi |
| `examples/udp_server` | UDP echo server on port 9000 |
| `examples/udp_client` | UDP client sending messages |

### Benchmarks

| Example | Description |
|---------|-------------|
| `bench/tcp_sim_bench.go` | Raw TCP connection benchmark (handshake + data) |
| `bench/network_sim_bench.go` | HTTP request/response benchmark over TCP |

## Building Examples

```bash
# Build all library-based examples
for ex in xdpserver xdpcurl httpserver_fasthttp httpserver_httphi \
    udp_server udp_client; do
    cd examples/$ex
    go build -o /tmp/afxdp-$ex .
    cd ../..
done
```

## Running Examples

### TCP/HTTP Server

```bash
# Run the server on eth0
sudo ./examples/xdpserver/xdpserver -i eth0

# Test with curl (from another machine)
curl http://10.10.20.2:8080/

# Test with the client example
sudo ./examples/xdpcurl/xdpcurl -i eth1 -target 10.10.20.2 -port 8080
```

### UDP Echo Server/Client

```bash
# Run the server on eth0
sudo ./examples/udp_server/udp_server -i eth0

# Send a test message from another machine
sudo ./examples/udp_client/udp_client -i eth1 -target 10.10.20.2 -port 9000 -msg "Hello from AF_XDP!"
```

## Network Configuration for Testing

For testing between two VMs:

| VM | Interface | IP Address | Subnet |
|----|-----------|------------|--------|
| Server | ens4 | 10.10.20.2 | /24 |
| Client | eth0 | 10.10.20.1 | /24 |

### Setting up network

```bash
# On server VM
sudo ip addr flush dev ens4
sudo ip addr add 10.10.20.2/24 dev ens4
sudo ip link set ens4 up

# On client VM
sudo ip addr flush dev eth0
sudo ip addr add 10.10.20.1/24 dev eth0
sudo ip link set eth0 up
```

### Verifying connectivity

```bash
# Test basic connectivity
ping 10.10.20.2  # from client
ping 10.10.20.1  # from server

# Capture packets to verify AF_XDP is receiving
sudo tcpdump -i ens4
```

## Debugging

### Enable verbose output

All library-based examples support the `-debug` flag to enable debug output:

```bash
sudo ./examples/xdpserver/xdpserver -i eth0 -debug
```

### Check interface state

```bash
# Verify IP address
ip addr show ens4

# Verify link state
ip link show ens4

# Check for offloading
ethtool -K ens4

# Capture packets
tcpdump -i ens4
```

## Performance

AF_XDP provides significantly lower latency and higher throughput compared to traditional socket APIs:

- ~100ns packet processing latency (vs ~1000ns for sockets)
- ~10M+ packets/sec per CPU core
- Zero-copy packet data
- Batch processing for efficient CPU utilization

## Benchmarks

The project includes two synthetic benchmarks that test the TCP/IP stack without requiring AF_XDP hardware or network interfaces. These benchmarks use `tcp.Handler` directly, simulating network communication in memory.

## Testing

To run unit tests:

```bash
make test
# or
go test -v -count=1 -tags tests ./...
```

Tests use the `tests` build tag, so they won't run during normal builds.

### TCP Handler Benchmark

Tests raw TCP connection performance (handshake + data exchange).

```bash
cd bench
go run -tags bench tcp_sim_bench.go -conns 10 -duration 5
```

Parameters:
- `-conns`: Number of concurrent connections (default: 10)
- `-duration`: Test duration in seconds (default: 5)

### HTTP over TCP Benchmark

Tests HTTP request/response throughput over simulated TCP connections.

```bash
cd bench
go run -tags bench network_sim_bench.go -conns 10 -duration 5
```

Parameters:
- `-conns`: Number of concurrent connections (default: 10)
- `-duration`: Test duration in seconds (default: 5)

Both benchmarks measure:
- Total requests completed
- Test duration
- Throughput (requests per second)

### Build Benchmarks

To build the benchmarks as standalone binaries:

```bash
cd bench
go build -tags bench -o tcp_sim_bench tcp_sim_bench.go
go build -tags bench -o network_sim_bench network_sim_bench.go
```

## License

MIT
