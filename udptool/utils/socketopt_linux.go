//go:build linux

package utils

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func SetSocketReusePort(network, address string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEADDR, 1) // supported since linux 2.4
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1) // supported since linux 3.9 and overrides SO_REUSEADDR
	})
}
