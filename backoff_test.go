//go:build tests

package afxdpnet

import (
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	factory := NewExponentialBackoff(100*time.Millisecond, 5*time.Second)
	strategy := factory()

	// n=0: 100ms
	if got := strategy(0); got != 100*time.Millisecond {
		t.Errorf("n=0: expected 100ms, got %v", got)
	}

	// n=1: 200ms
	if got := strategy(1); got != 200*time.Millisecond {
		t.Errorf("n=1: expected 200ms, got %v", got)
	}

	// n=2: 400ms
	if got := strategy(2); got != 400*time.Millisecond {
		t.Errorf("n=2: expected 400ms, got %v", got)
	}

	// n=3: 800ms
	if got := strategy(3); got != 800*time.Millisecond {
		t.Errorf("n=3: expected 800ms, got %v", got)
	}

	// n=4: 1600ms
	if got := strategy(4); got != 1600*time.Millisecond {
		t.Errorf("n=4: expected 1600ms, got %v", got)
	}

	// n=5: 3200ms
	if got := strategy(5); got != 3200*time.Millisecond {
		t.Errorf("n=5: expected 3200ms, got %v", got)
	}

	// n=6: capped at 5s
	if got := strategy(6); got != 5*time.Second {
		t.Errorf("n=6: expected 5s (capped), got %v", got)
	}

	// n=7: still capped
	if got := strategy(7); got != 5*time.Second {
		t.Errorf("n=7: expected 5s (capped), got %v", got)
	}
}

func TestLinearBackoff(t *testing.T) {
	factory := NewLinearBackoff(100*time.Millisecond, 50*time.Millisecond, 500*time.Millisecond)
	strategy := factory()

	// n=0: 100ms
	if got := strategy(0); got != 100*time.Millisecond {
		t.Errorf("n=0: expected 100ms, got %v", got)
	}

	// n=1: 150ms
	if got := strategy(1); got != 150*time.Millisecond {
		t.Errorf("n=1: expected 150ms, got %v", got)
	}

	// n=2: 200ms
	if got := strategy(2); got != 200*time.Millisecond {
		t.Errorf("n=2: expected 200ms, got %v", got)
	}

	// n=3: 250ms
	if got := strategy(3); got != 250*time.Millisecond {
		t.Errorf("n=3: expected 250ms, got %v", got)
	}

	// n=4: 300ms
	if got := strategy(4); got != 300*time.Millisecond {
		t.Errorf("n=4: expected 300ms, got %v", got)
	}

	// n=5: 350ms
	if got := strategy(5); got != 350*time.Millisecond {
		t.Errorf("n=5: expected 350ms, got %v", got)
	}

	// n=6: 400ms
	if got := strategy(6); got != 400*time.Millisecond {
		t.Errorf("n=6: expected 400ms, got %v", got)
	}

	// n=7: 450ms
	if got := strategy(7); got != 450*time.Millisecond {
		t.Errorf("n=7: expected 450ms, got %v", got)
	}

	// n=8: 500ms (exactly max)
	if got := strategy(8); got != 500*time.Millisecond {
		t.Errorf("n=8: expected 500ms, got %v", got)
	}

	// n=9: capped at 500ms
	if got := strategy(9); got != 500*time.Millisecond {
		t.Errorf("n=9: expected 500ms (capped), got %v", got)
	}
}

func TestConstantBackoff(t *testing.T) {
	factory := NewConstantBackoff(100 * time.Millisecond)
	strategy := factory()

	// All retries should have the same backoff
	for n := uint(0); n < 10; n++ {
		if got := strategy(n); got != 100*time.Millisecond {
			t.Errorf("n=%d: expected 100ms, got %v", n, got)
		}
	}
}
