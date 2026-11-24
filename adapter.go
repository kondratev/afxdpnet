//go:build linux && !tinygo

package afxdpnet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"net/netip"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/atoonk/go-afxdp"
	"github.com/vishvananda/netlink"
)

const safamily_hw6 = 1

// XDPDevice is an AF_XDP-based network interface adapter for lneto.
type XDPDevice struct {
	fleet   *afxdp.Fleet
	iface   string
	index   int
	sockets []*afxdp.Socket
}

// NewXDPDevice opens an AF_XDP interface and returns a device ready for use.
// It uses the Fleet API which auto-selects XDP mode and handles queue binding.
func NewXDPDevice(name string) (*XDPDevice, error) {
	// Use Fleet API with MatchAll filter to capture all packets
	fleet, err := afxdp.Open(name,
		afxdp.WithFilter(afxdp.MatchAll()),
	)
	if err != nil {
		return nil, fmt.Errorf("opening AF_XDP interface %q: %w", name, err)
	}

	if info, err := fleet.Info(); err == nil {
		Printf("[afxdp] %s: %s", name, info.String())
	}

	// Native XDP attach bounces the link on many NICs (~10s on ixgbe); wait
	// for it so the quiet start doesn't read as "receiving nothing".
	Printf("waiting for the link to come up...")
	if fleet.WaitLinkUp(15 * time.Second) {
		Printf("link up")
	} else {
		Printf("warning: link not up after 15s; continuing anyway")
	}

	return &XDPDevice{
		fleet:   fleet,
		iface:   name,
		index:   -1,
		sockets: fleet.Sockets(),
	}, nil
}

// socket returns the first available AF_XDP socket.
func (xdp *XDPDevice) socket() (*afxdp.Socket, error) {
	sockets := xdp.fleet.Sockets()
	if len(sockets) == 0 {
		return nil, errors.New("no AF_XDP sockets available")
	}
	return sockets[0], nil
}

// Sockets returns all available AF_XDP sockets for direct access.
func (xdp *XDPDevice) Sockets() []*afxdp.Socket {
	return xdp.sockets
}

// fill provides buffers to the kernel. Must be called before Poll/Receive.
func (xdp *XDPDevice) fill() {
	xsk, err := xdp.socket()
	if err != nil {
		return
	}
	xsk.Fill(xsk.NumFreeFillSlots())
}

// Close closes the AF_XDP device and cleans up resources.
func (xdp *XDPDevice) Close() error {
	return xdp.fleet.Close()
}

// HardwareAddress6 returns the MAC address of the interface.
func (xdp *XDPDevice) HardwareAddress6() (hw [6]byte, err error) {
	iface, err := interfaceByName(xdp.iface)
	if err != nil {
		return hw, err
	}
	xdp.index = iface.Attrs().Index
	hwSlice := iface.Attrs().HardwareAddr
	if len(hwSlice) != 6 {
		return hw, fmt.Errorf("unexpected MAC address length: %d", len(hwSlice))
	}
	copy(hw[:], hwSlice)
	return hw, nil
}

// IPMask returns the IP address and subnet mask of the interface.
func (xdp *XDPDevice) IPMask() (netip.Prefix, error) {
	sock, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_IP)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("creating socket for IP query: %w", err)
	}
	defer syscall.Close(sock)

	return getSocketMask(sock, xdp.iface)
}

// MTU returns the MTU of the interface.
func (xdp *XDPDevice) MTU() (int, error) {
	sock, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_IP)
	if err != nil {
		return 0, fmt.Errorf("creating socket for MTU query: %w", err)
	}
	defer syscall.Close(sock)

	return getSocketMTU(sock, xdp.iface)
}

// Poll waits up to timeout for the AF_XDP socket to have data available.
// Returns true if data is available, false if timeout was reached.
func (xdp *XDPDevice) Poll(timeout time.Duration) (bool, error) {
	xsk, err := xdp.socket()
	if err != nil {
		return false, err
	}

	// Fill before polling
	xdp.fill()

	n, err := xsk.Poll(timeout)
	if err != nil {
		return false, fmt.Errorf("polling AF_XDP socket: %w", err)
	}

	return n > 0, nil
}

// ---- socket helpers (reused from tap.go pattern) ----

func interfaceByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

type ifreq struct {
	Name [syscall.IFNAMSIZ]byte
	Data [64]byte
}

func makeifreq(name string) ifreq {
	var ifr ifreq
	copy(ifr.Name[:], name)
	return ifr
}

func (ifr *ifreq) ptr() unsafe.Pointer { return unsafe.Pointer(ifr) }

func ioctl(fd int, request uintptr, argp unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(argp))
	if errno != 0 {
		return os.NewSyscallError("ioctl", errno)
	}
	return nil
}

func getSocketHW(sockfd int, ifaceName string) (hw [6]byte, err error) {
	ifr := makeifreq(ifaceName)
	err = ioctl(sockfd, syscall.SIOCGIFHWADDR, ifr.ptr())
	if err != nil {
		return hw, err
	}
	sa_family := *(*uint16)(unsafe.Pointer(&ifr.Data[0]))
	if sa_family != safamily_hw6 {
		return hw, fmt.Errorf("expecting sa_family=1 got %d", sa_family)
	}
	copy(hw[:], ifr.Data[2:])
	return hw, nil
}

func setSocketHW(sockfd int, ifaceName string, hw [6]byte) error {
	ifr := makeifreq(ifaceName)
	*(*uint16)(unsafe.Pointer(&ifr.Data[0])) = safamily_hw6
	copy(ifr.Data[2:], hw[:])
	err := ioctl(sockfd, syscall.SIOCSIFHWADDR, ifr.ptr())
	if err != nil {
		return fmt.Errorf("setting hw addr: %w", err)
	}
	return nil
}

func getSocketMTU(sockfd int, ifaceName string) (int, error) {
	ifr := makeifreq(ifaceName)
	err := ioctl(sockfd, syscall.SIOCGIFMTU, ifr.ptr())
	if err != nil {
		return 0, err
	}
	mtu := *(*int32)(unsafe.Pointer(&ifr.Data[0]))
	return int(mtu), nil
}

func getSocketMask(sockfd int, ifaceName string) (netip.Prefix, error) {
	addrp, err := getSocketIP(sockfd, ifaceName)
	if err != nil {
		return netip.Prefix{}, err
	}
	ifr := makeifreq(ifaceName)
	err = ioctl(sockfd, syscall.SIOCGIFNETMASK, ifr.ptr())
	if err != nil {
		return netip.Prefix{}, err
	}
	addr32 := binary.BigEndian.Uint32(ifr.Data[4:8])
	cidr := bits.OnesCount32(addr32)
	return netip.PrefixFrom(addrp.Addr(), cidr), nil
}

func getSocketIP(sockfd int, ifaceName string) (addrp netip.AddrPort, err error) {
	ifr := makeifreq(ifaceName)
	err = ioctl(sockfd, syscall.SIOCGIFADDR, ifr.ptr())
	if err != nil {
		return netip.AddrPort{}, err
	}
	safamily := *(*uint16)(unsafe.Pointer(&ifr.Data[0]))
	port := *(*uint16)(unsafe.Pointer(&ifr.Data[2]))
	switch safamily {
	case 2:
		addr, _ := netip.AddrFromSlice(ifr.Data[4:8])
		addrp = netip.AddrPortFrom(addr, port)
	default:
		return addrp, fmt.Errorf("unsupported IP addr sa_family=%d", safamily)
	}
	return addrp, nil
}
