module github.com/kondratev/afxdpnet/examples/httpserver_fasthttp

go 1.27.4

require (
	github.com/kondratev/afxdpnet v0.0.0
	github.com/soypat/lneto v0.0.0
	github.com/valyala/fasthttp v1.80.0
)

require (
	github.com/atoonk/go-afxdp v0.12.0 // indirect
	github.com/cilium/ebpf v0.16.0 // indirect
	github.com/klauspost/compress v1.20.4 // indirect
	github.com/molecule-man/go-brrr v1.3.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/vishvananda/netlink v1.3.1 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2 // indirect
	golang.org/x/sys v0.50.0 // indirect
)


replace (
	github.com/kondratev/afxdpnet => ../../
	github.com/soypat/lneto => github.com/kondratev/lneto v0.0.0-20261125233610-f9d07a77c6c5
)
