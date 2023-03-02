//go:build windows

package utils

import (
	"syscall"

	"golang.org/x/sys/windows"
)

func SetSocketReusePort(network, address string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		var val byte = 1
		windows.Setsockopt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, &val, 1)
	})
}
