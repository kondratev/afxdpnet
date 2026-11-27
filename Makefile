# AF_XDP Network Stack with lneto

EX = xdpserver_lib xdpcurl_lib httpserver_fasthttp httpserver_httphi udp_server udp_client xdpserver xdpcurl http_benchmark

all: build

build:
	@for ex in $(EX); do \
		echo "Building examples/$$ex..."; \
		(cd examples/$$ex && go build -o /tmp/afxdp-$$ex .) || exit 1; \
	done
	@echo "All examples built to /tmp/"

test:
	@echo "Running tests..."
	@go test -v -count=1 -tags tests ./...

bench:
	@echo "=== TCP Benchmark (small) ==="
	(cd bench && go run -tags bench tcp_sim_bench.go -conns 10 -duration 5)
	@echo ""
	@echo "=== TCP Benchmark (default) ==="
	(cd bench && go run -tags bench tcp_sim_bench.go -conns 50 -duration 10)
	@echo ""
	@echo "=== TCP Benchmark (large) ==="
	(cd bench && go run -tags bench tcp_sim_bench.go -conns 100 -duration 15)
	@echo ""
	@echo "=== Network Benchmark (small) ==="
	(cd bench && go run -tags bench network_sim_bench.go -conns 10 -duration 5)
	@echo ""
	@echo "=== Network Benchmark (default) ==="
	(cd bench && go run -tags bench network_sim_bench.go -conns 50 -duration 10)
	@echo ""
	@echo "=== Network Benchmark (large) ==="
	(cd bench && go run -tags bench network_sim_bench.go -conns 100 -duration 15)
	@echo ""
	@echo "All benchmarks completed"

clean:
	rm -f /tmp/afxdp-*

.PHONY: all build test clean bench
