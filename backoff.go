package afxdpnet

import (
	"math"
	"time"

	"github.com/soypat/lneto"
)

// NewExponentialBackoff returns a new exponential backoff strategy factory.
// The backoff duration grows exponentially: initial * 2^n, capped at max.
//
// Example usage:
//
//	stack, err := afxdpnet.New("eth0", afxdpnet.Config{
//	    IP:            netip.MustParseAddr("10.10.20.2"),
//	    Subnet:        netip.MustParsePrefix("10.10.20.0/24"),
//	    NewBackoff:    afxdpnet.NewExponentialBackoff(100*time.Millisecond, 5*time.Second),
//	})
func NewExponentialBackoff(initial, max time.Duration) func() lneto.BackoffStrategy {
	return func() lneto.BackoffStrategy {
		return func(n uint) time.Duration {
			// Compute initial * 2^n with overflow protection
			mult := math.Pow(2, float64(n))
			backoff := time.Duration(float64(initial) * mult)
			if backoff > max {
				return max
			}
			return backoff
		}
	}
}

// NewLinearBackoff returns a new linear backoff strategy factory.
// The backoff duration grows linearly: initial + n * increment, capped at max.
//
// Example usage:
//
//	stack, err := afxdpnet.New("eth0", afxdpnet.Config{
//	    IP:            netip.MustParseAddr("10.10.20.2"),
//	    Subnet:        netip.MustParsePrefix("10.10.20.0/24"),
//	    NewBackoff:    afxdpnet.NewLinearBackoff(100*time.Millisecond, 50*time.Millisecond, 5*time.Second),
//	})
func NewLinearBackoff(initial, increment, max time.Duration) func() lneto.BackoffStrategy {
	return func() lneto.BackoffStrategy {
		return func(n uint) time.Duration {
			backoff := initial + time.Duration(n)*increment
			if backoff > max {
				return max
			}
			return backoff
		}
	}
}

// NewConstantBackoff returns a new constant backoff strategy factory.
// The backoff duration is always fixed at the specified value, regardless of n.
//
// Example usage:
//
//	stack, err := afxdpnet.New("eth0", afxdpnet.Config{
//	    IP:            netip.MustParseAddr("10.10.20.2"),
//	    Subnet:        netip.MustParsePrefix("10.10.20.0/24"),
//	    NewBackoff:    afxdpnet.NewConstantBackoff(100*time.Millisecond),
//	})
func NewConstantBackoff(duration time.Duration) func() lneto.BackoffStrategy {
	return func() lneto.BackoffStrategy {
		return func(n uint) time.Duration {
			return duration
		}
	}
}
