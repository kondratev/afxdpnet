// Package afxdpnet provides AF_XDP network stack initialization using lneto.
//
// This package encapsulates the common AF_XDP setup code and provides a simple
// API for creating a lneto stack.
//
// Example usage:
//
//	stack, err := afxdpnet.New("eth0", afxdpnet.Config{
//		IP:     netip.MustParseAddr("10.10.20.1"),
//		Subnet: netip.MustParsePrefix("10.10.20.0/24"),
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer stack.Close()
//
//	// Start the packet processing loop
//	stack.Start()
//
//	// Use standard Go net types
//	conn, err := stack.DialTCP("10.10.20.2:8080")
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Requirements:
//   - Linux kernel with AF_XDP support (4.18+)
//   - go-afxdp library: go get github.com/atoonk/go-afxdp
//   - CAP_NET_RAW (root or sudo)
//   - Checksum offloading disabled: sudo ethtool -K eth0 tx off
package afxdpnet

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"net"
	"net/netip"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/soypat/lneto"
	"github.com/soypat/lneto/dns"
	"github.com/soypat/lneto/x/xnet"

	"github.com/atoonk/go-afxdp"
)

const (
	sockSTREAM = syscall.SOCK_STREAM
	sockDGRAM  = syscall.SOCK_DGRAM
)

// Config holds configuration for the AF_XDP network stack.
type Config struct {
	// IP is the local IP address to use.
	IP netip.Addr
	// Subnet is the local subnet prefix.
	Subnet netip.Prefix
	// Router is the default gateway IP (optional, can be zero value).
	Router netip.Addr
	// Broadcast is the broadcast address (optional, will be calculated if zero).
	Broadcast netip.Addr
	// PassivePeers is the number of peer MAC addresses to learn passively.
	// Required for TCP server connections. Defaults to 16 if zero.
	PassivePeers int
	// NewBackoff is the backoff strategy factory for TCP retries.
	// If nil, uses exponential backoff (100ms initial, 5s max).
	NewBackoff func() lneto.BackoffStrategy
}

// Stack is an AF_XDP network stack.
type Stack struct {
	async   *xnet.StackAsync
	pool    *xnet.TCPPool
	dev     *XDPDevice
	backoff func(n uint) time.Duration
	goStack xnet.StackGo
	cancel  context.CancelFunc
}

// New creates a new AF_XDP network stack on the specified interface.
// The process loop does not start automatically - call Start() after
// registering any listeners.
func New(iface string, cfg Config) (*Stack, error) {
	// Open AF_XDP device using the adapter
	Printf("[afxdp] Opening device on interface %q...\n", iface)
	dev, err := NewXDPDevice(iface)
	if err != nil {
		return nil, fmt.Errorf("opening AF_XDP device: %w", err)
	}
	Println("[afxdp] Device opened successfully")

	// Get interface MAC address
	mac, err := dev.HardwareAddress6()
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("getting MAC address: %w", err)
	}
	Printf("MAC: %s\n", net.HardwareAddr(mac[:]).String())

	// Get MTU
	mtu, err := dev.MTU()
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("getting MTU: %w", err)
	}
	Printf("MTU: %d\n", mtu)

	Printf("Static IP: %s, Subnet: %s\n", cfg.IP.String(), cfg.Subnet.String())

	// Calculate broadcast if not provided
	var broadcast netip.Addr
	if cfg.Broadcast.Is4() || cfg.Broadcast.Is6() {
		broadcast = cfg.Broadcast
	} else {
		ones := cfg.Subnet.Bits()
		if ones == 8 {
			broadcast = netip.AddrFrom4([4]byte{cfg.IP.AsSlice()[0], 255, 255, 255})
		} else if ones == 16 {
			broadcast = netip.AddrFrom4([4]byte{cfg.IP.AsSlice()[0], cfg.IP.AsSlice()[1], 255, 255})
		} else if ones == 24 {
			broadcast = netip.AddrFrom4([4]byte{cfg.IP.AsSlice()[0], cfg.IP.AsSlice()[1], cfg.IP.AsSlice()[2], 255})
		}
	}

	Printf("Broadcast: %s\n", broadcast.String())

	// Initialize lneto stack with static IP
	Println("[lneto] Initializing stack...")
	stack := &xnet.StackAsync{}

	// Defaults
	passivePeers := cfg.PassivePeers
	if passivePeers == 0 {
		passivePeers = 16
	}

	// Backoff strategy: use provided or default to exponential
	var backoff lneto.BackoffStrategy
	if cfg.NewBackoff != nil {
		backoff = cfg.NewBackoff()
	} else {
		expBackoff := NewExponentialBackoff(100*time.Millisecond, 5*time.Second)
		backoff = expBackoff()
	}

	err = stack.Reset(xnet.StackConfig{
		Hostname:          "afxdpnet",
		RandSeed:          time.Now().Unix(),
		Entropy:           rand.Read,
		HardwareAddress:   mac,
		MTU:               uint16(mtu),
		MaxActiveTCPPorts: 30000,
		MaxActiveUDPPorts: 10,
		PassivePeers:      passivePeers,
	})
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("initializing lneto stack: %w", err)
	}

	// Set IP address, subnet, gateway, broadcast via DHCPResults
	results := &xnet.DHCPResults{
		AssignedAddr4: [4]byte(cfg.IP.AsSlice()),
		Subnet:        cfg.Subnet,
		Router:        cfg.Router,
		BroadcastAddr: broadcast,
	}
	if err := stack.AssimilateDHCPResults(results); err != nil {
		dev.Close()
		return nil, fmt.Errorf("setting network config: %w", err)
	}
	Printf("[lneto] IP=%s subnet=%s router=%s broadcast=%s\n",
		cfg.IP.String(), cfg.Subnet.String(), cfg.Router.String(), broadcast.String())

	// Create TCP pool with larger defaults
	poolSize := uint16(2000)
	queueSize := 150
	// Use larger buffers for better throughput (4x MTU)
	txSize := mtu * 4
	rxSize := mtu * 4
	pool, err := stack.NewTCPPool(xnet.TCPPoolConfig{
		PoolSize:           poolSize,
		QueueSize:          queueSize,
		TxBufSize:          txSize,
		RxBufSize:          rxSize,
		EstablishedTimeout: 30 * time.Second,
		ClosingTimeout:     500 * time.Millisecond,
		NewBackoff: func() lneto.BackoffStrategy {
			return backoff
		},
	})
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("creating TCP pool: %w", err)
	}

	// Create StackGo wrapper for standard Go compatibility
	goStack := stack.StackGo(backoff, xnet.StackGoConfig{
		ListenerPoolConfig: xnet.TCPPoolConfig{
			PoolSize:           poolSize,
			QueueSize:          queueSize,
			TxBufSize:          txSize,
			RxBufSize:          rxSize,
			EstablishedTimeout: 30 * time.Second,
			ClosingTimeout:     500 * time.Millisecond,
			NewBackoff: func() lneto.BackoffStrategy {
				return backoff
			},
		},
	})

	// Don't start loop yet - caller must call Start() after registering listeners
	s := &Stack{
		async:   stack,
		pool:    pool,
		dev:     dev,
		backoff: backoff,
		goStack: goStack,
	}

	return s, nil
}

// Start begins the packet processing loop. Call this after registering
// any listeners. The provided context controls the lifetime of the loop.
func (s *Stack) Start(ctx context.Context) {
	Println("[loop] Starting packet processing loop...")
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	for _, xsk := range s.dev.Sockets() {
		go s.processTx(ctx, xsk)
		go s.processRx(ctx, xsk)
	}
}

func (s *Stack) processTx(ctx context.Context, xsk *afxdp.Socket) {
	Println("[loop] ProcessTx loop started")
	buf := make([]byte, math.MaxUint16)

	for {
		select {
		case <-ctx.Done():
			Println("[loop] ProcessTx context done")
			return
		default:
			xsk.Complete(xsk.NumCompleted())
			xsk.Fill(xsk.NumFreeFillSlots()) // give the kernel buffers
			n, err := xsk.Poll(-1)           // block until packets arrive
			if err != nil {
				Printf("[loop] ProcessTx poll error: %v\n", err)
				return
			}

			// Receive packets
			descs := xsk.Receive(n)

			for _, desc := range descs {
				frame := xsk.GetFrame(desc)
				avail := len(frame)
				if avail > len(buf) {
					avail = len(buf)
				}
				copy(buf, frame[:avail])

				Printf("[loop] Received %d bytes\n", avail)

				// Feed packet to lneto stack
				s.async.IngressEthernet(buf[:avail])
			}

			xsk.Recycle(descs) // return frames to be filled again

			// Yield to other goroutines
			runtime.Gosched()
		}
	}
}

func (s *Stack) processRx(ctx context.Context, xsk *afxdp.Socket) {
	Println("[loop] ProcessRx loop started")
	buf := make([]byte, math.MaxUint16)

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			Println("[loop] ProcessRx context done")
			return
		case <-ticker.C:
			nwrite, err := s.async.EgressEthernet(buf[:])
			if err != nil {
				Printf("Egress error: %w", err)
				continue
			}
			if nwrite > 0 {
				n, err := xsk.SendBatch([][]byte{buf[:nwrite]})
				if err != nil {
					Printf("[afxdp] TX SendBatch error: %v\n", err)
					return
				}
				if n < 1 {
					Printf("[afxdp] TX SendBatch returned n=0 (queue full?)\n")
					return
				}
				Printf("[afxdp] TX: sent %d bytes\n", nwrite)
			}
		}
	}
}

// Pool returns the TCP pool.
func (s *Stack) Pool() *xnet.TCPPool {
	return s.pool
}

// DialTCP creates a TCP connection to the given address.
// Returns a standard Go net.Conn.
func (s *Stack) DialTCP(addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	raddr := netip.AddrPortFrom(netip.MustParseAddr(host), uint16(port))
	conn, err := s.goStack.SocketNetip(context.Background(), "tcp", syscall.AF_INET, sockSTREAM, netip.AddrPort{}, raddr)
	if err != nil {
		return nil, err
	}
	return conn.(net.Conn), nil
}

// DialTCPAddr creates a TCP connection to the given address using netip.
// Returns a standard Go net.Conn.
func (s *Stack) DialTCPAddr(raddr netip.AddrPort) (net.Conn, error) {
	conn, err := s.goStack.SocketNetip(context.Background(), "tcp", syscall.AF_INET, sockSTREAM, netip.AddrPort{}, raddr)
	if err != nil {
		return nil, err
	}
	return conn.(net.Conn), nil
}

// ListenTCP listens on the given port for TCP connections.
// Returns a standard Go net.Listener.
func (s *Stack) ListenTCP(port uint16) (net.Listener, error) {
	laddr := netip.AddrPortFrom(netip.IPv4Unspecified(), port)
	conn, err := s.goStack.SocketNetip(context.Background(), "tcp", syscall.AF_INET, sockSTREAM, laddr, netip.AddrPort{})
	if err != nil {
		return nil, err
	}
	return conn.(net.Listener), nil
}

// ListenUDP listens on the given port for UDP connections.
// Returns a standard Go net.PacketConn.
func (s *Stack) ListenUDP(port uint16) (net.PacketConn, error) {
	laddr := netip.AddrPortFrom(netip.IPv4Unspecified(), port)
	conn, err := s.goStack.SocketNetip(context.Background(), "udp", syscall.AF_INET, sockDGRAM, laddr, netip.AddrPort{})
	if err != nil {
		return nil, err
	}
	return conn.(net.PacketConn), nil
}

// DialUDP creates a UDP connection to the given remote address.
// Returns a standard Go net.Conn.
func (s *Stack) DialUDP(remote string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(remote)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	raddr := netip.AddrPortFrom(netip.MustParseAddr(host), uint16(port))
	conn, err := s.goStack.SocketNetip(context.Background(), "udp", syscall.AF_INET, sockDGRAM, netip.AddrPort{}, raddr)
	if err != nil {
		return nil, err
	}
	return conn.(net.Conn), nil
}

// ResolveHardwareAddress6 starts ARP resolution for an IPv4 address.
func (s *Stack) ResolveHardwareAddress6(addr netip.Addr) error {
	return s.async.StartResolveHardwareAddress6(addr)
}

// ResultResolveHardwareAddress6 gets the result of ARP resolution.
func (s *Stack) ResultResolveHardwareAddress6(addr netip.Addr) ([6]byte, error) {
	return s.async.ResultResolveHardwareAddress6(addr)
}

// Addr4 returns the local IPv4 address.
func (s *Stack) Addr4() [4]byte {
	return s.async.Addr4()
}

// HardwareAddr returns the local MAC address.
func (s *Stack) HardwareAddr() [6]byte {
	return s.async.HardwareAddr()
}

// Close shuts down the network stack and releases resources.
func (s *Stack) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return s.dev.Close()
}

// Socket is a low-level socket operation that provides full control.
// This is for advanced users who need fine-grained control.
func (s *Stack) Socket(ctx context.Context, network string, family, sotype int, laddr, raddr net.Addr) (any, error) {
	return s.goStack.Socket(ctx, network, family, sotype, laddr, raddr)
}

// SocketNetip is a low-level socket operation using netip.AddrPort.
func (s *Stack) SocketNetip(ctx context.Context, network string, family, sotype int, laddr, raddr netip.AddrPort) (any, error) {
	return s.goStack.SocketNetip(ctx, network, family, sotype, laddr, raddr)
}

// ParseAddrPort parses a "host:port" string into a netip.AddrPort.
// Useful for constructing addresses for SocketNetip calls.
func ParseAddrPort(addr string) (netip.AddrPort, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return netip.AddrPort{}, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return netip.AddrPort{}, err
	}
	ip := netip.MustParseAddr(host)
	return netip.AddrPortFrom(ip, uint16(port)), nil
}

// Ping sends an ICMP echo request to the given address and waits for a response.
// Returns the round-trip time if successful, or an error if the target is unreachable.
func (s *Stack) Ping(addr string, timeout time.Duration) (time.Duration, error) {
	ip := netip.MustParseAddr(addr)
	blk := s.async.StackBlocking(s.backoff)
	return blk.DoPing(ip, timeout)
}

// DHCPRequest performs a DHCPv4 request and configures the network stack
// with the obtained IP address, subnet, gateway, and broadcast address.
// If reqAddr is zero, DHCP will request any available address.
func (s *Stack) DHCPRequest(reqAddr netip.Addr, timeout time.Duration) (*xnet.DHCPResults, error) {
	if reqAddr.Is4() {
		blk := s.async.StackBlocking(s.backoff)
		return blk.DoDHCPv4([4]byte(reqAddr.AsSlice()), timeout)
	}
	return nil, lneto.ErrInvalidAddr
}

// LookupIP resolves a hostname to an IP address using DNS.
func (s *Stack) LookupIP(host string) ([]net.IP, error) {
	var addrs [1]netip.Addr
	blk := s.async.StackBlocking(s.backoff)
	name := dns.MustNewName(host)
	n, err := blk.DoLookupIP(addrs[:], name, 5*time.Second)
	if err != nil {
		return nil, err
	}
	var result []net.IP
	for i := 0; i < n; i++ {
		result = append(result, net.IP(addrs[i].AsSlice()))
	}
	return result, nil
}
