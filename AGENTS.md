# AF_XDP Network Stack with lneto

High-performance network stack using AF_XDP sockets for packet I/O, integrated with the lneto TCP/IP stack. Provides a simple API for creating TCP/UDP listeners and connections.

## Architecture

```
Application → afxdpnet (lib) → lneto TCP/IP → AF_XDP adapter → Linux AF_XDP
```

- **afxdpnet**: Network stack management (go-afxdp, netlink integration)
- **lneto/x/xnet**: TCP/IP stack with passive peer learning
- **go-afxdp**: AF_XDP socket management (Fleet API)
- **Linux AF_XDP**: Kernel-level XDP sockets

## Requirements

- Linux kernel 4.18+ with AF_XDP support
- go-afxdp library
- CAP_NET_RAW and CAP_SYS_ADMIN (run as root or with sudo)
- Checksum offloading disabled: `sudo ethtool -K eth0 tx off rx off`

## AI Agent Guidelines

### MCP Server Usage

- **golsp**: For code intelligence (go-to-definition, references, diagnostics, type information)
- **godig**: For package/module documentation and API reference
- **duckduckgo**: For web searches when documentation is needed
- **memory**: For storing and retrieving project-specific knowledge during development

### Code Style and Conventions

- Follow Go style conventions (gofmt)
- Use meaningful variable and function names
- Write tests for new functionality
- Document public APIs

### Project Structure

- **Main module**: github.com/kondratev/afxdpnet
- **Library**: afxdpnet.go, adapter.go, backoff.go — network stack management
- **Examples**: Separate go.mod files, build from within their directories
- **Benchmarks**: Use `bench` build tag
- **Tests**: Use `tests` build tag

### Building

```bash
make build    # Build all examples
make test     # Run tests
make bench    # Run benchmarks
```

## API

### Creating a Network Stack

```go
import "github.com/kondratev/afxdpnet"

stack, err := afxdpnet.New("eth0", afxdpnet.WithStaticIP("10.10.20.2"))
if err != nil {
    log.Fatal(err)
}
defer stack.Close()
```

### TCP Listener

```go
listener, err := stack.ListenTCP("tcp", "10.10.20.2:8080")
if err != nil {
    log.Fatal(err)
}
defer listener.Close()

for {
    conn, err := listener.Accept()
    if err != nil {
        break
    }
    go handleConnection(conn)
}
```

### TCP Client

```go
conn, err := stack.DialTCP("tcp", "10.10.20.1:8080")
if err != nil {
    log.Fatal(err)
}
defer conn.Close()
```

### UDP Socket

```go
conn, err := stack.ListenUDP("10.10.20.2:5353")
if err != nil {
    log.Fatal(err)
}
defer conn.Close()
```

## Examples

- **xdpserver_lib**: Basic TCP server with lneto
- **xdpcurl_lib**: HTTP client using lneto
- **httpserver_fasthttp**: HTTP server with fasthttp
- **httpserver_httphi**: HTTP server with httphi
- **udp_server**: Basic UDP server
- **udp_client**: Basic UDP client
- **xdpserver**: Simple TCP echo server
- **xdpcurl**: HTTP client example
- **http_benchmark**: HTTP benchmark tool

## Testing

```bash
go test -tags tests ./...
```

## Benchmarks

```bash
go run -tags bench ./bench/tcp_sim_bench.go
go run -tags bench ./bench/network_sim_bench.go
```

## Configuration Options

- `WithStaticIP(ip string)` - Static IP address
- `WithGateway(gateway string)` - Default gateway
- `WithPort(port uint16)` - AF_XDP port
- `WithTxQueueSize(size int)` - TX queue size
- `WithRxQueueSize(size int)` - RX queue size
- `WithRingSize(size int)` - AF_XDP ring size
- `WithTimeout(timeout time.Duration)` - Socket timeout

## Troubleshooting

- Interface already exists - Remove it first: `sudo ip link del afxdp0`
- AF_XDP not supported - Check kernel version and AF_XDP configuration
- Checksum errors - Disable hardware checksums
- Permission errors - Run with sudo or appropriate capabilities