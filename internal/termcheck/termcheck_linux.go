//go:build linux

package termcheck

import (
	"syscall"
	"unsafe"
)

const ioctlReadTermios = syscall.TCGETS

func IsTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL, fd, ioctlReadTermios,
		uintptr(unsafe.Pointer(&t)), 0, 0, 0,
	)
	return err == 0
}
