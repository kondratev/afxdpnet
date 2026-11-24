//go:build !debug

package afxdpnet

// Printf is a no-op in release builds
func Printf(format string, args ...interface{}) {}

// Println is a no-op in release builds
func Println(args ...interface{}) {}
