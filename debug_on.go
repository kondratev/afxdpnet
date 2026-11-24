//go:build debug

package afxdpnet

import "log"

// Printf prints a debug message in debug builds
func Printf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// Println prints a debug message in debug builds
func Println(args ...interface{}) {
	log.Println(args...)
}
