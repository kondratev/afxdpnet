//go:build linux && !tinygo

// http_benchmark is a simple HTTP load testing tool that makes concurrent
// HTTP requests to a server and reports throughput and latency.
//
// Usage:
//   ./http_benchmark -url http://10.10.20.2:8080/ -rate 100 -duration 10s
package main

import (
	"flag"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kondratev/afxdpnet"
)

var DebugPrintf = afxdpnet.Printf
var DebugPrintln = afxdpnet.Println

var (
	successCount int64
	failCount    int64
	totalLatency int64
)

func main() {
	var url string
	var rate int
	var duration time.Duration

	flag.StringVar(&url, "url", "http://10.10.20.2:8080/", "URL to test")
	flag.IntVar(&rate, "rate", 100, "Requests per second")
	flag.DurationVar(&duration, "duration", 10*time.Second, "Test duration")
	flag.Parse()

	DebugPrintf("Benchmarking %s\n", url)
	DebugPrintf("Rate: %d req/s, Duration: %s\n\n", rate, duration)

	// Warm up connection
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Run benchmark
	start := time.Now()
	var wg sync.WaitGroup
	interval := time.Second / time.Duration(rate)

	var ticker *time.Ticker
	if rate > 0 {
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}

	// Run for specified duration
	done := time.After(duration)

	for {
		select {
		case <-done:
			goto finish
		default:
		}

		// Send burst of requests based on rate
		for i := 0; i < rate; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reqStart := time.Now()

				resp, err := client.Get(url)
				if err != nil {
					atomic.AddInt64(&failCount, 1)
					return
				}
				defer resp.Body.Close()
				io.Copy(io.Discard, resp.Body)

				latency := time.Since(reqStart).Microseconds()
				atomic.AddInt64(&totalLatency, latency)

				if resp.StatusCode == 200 {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&failCount, 1)
				}
			}()
		}

		// Wait for rate interval
		if ticker != nil {
			<-ticker.C
		}
	}

finish:
	wg.Wait()
	totalElapsed := time.Since(start).Seconds()

	success := atomic.LoadInt64(&successCount)
	fails := atomic.LoadInt64(&failCount)
	total := success + fails
	avgLatency := int64(0)
	if total > 0 {
		avgLatency = atomic.LoadInt64(&totalLatency) / total
	}

	DebugPrintf("\nResults:\n")
	DebugPrintf("Total requests: %d\n", total)
	DebugPrintf("Success: %d\n", success)
	DebugPrintf("Failed: %d\n", fails)
	DebugPrintf("Duration: %.2fs\n", totalElapsed)
	DebugPrintf("Throughput: %.2f req/s\n", float64(success)/totalElapsed)
	if total > 0 {
		DebugPrintf("Avg latency: %d µs\n", avgLatency)
	}
}
