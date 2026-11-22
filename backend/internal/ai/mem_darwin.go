package ai

import "golang.org/x/sys/unix"

// TotalRAM returns the computer's total memory in bytes, or 0 if it cannot be read.
func TotalRAM() uint64 {
	n, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return n
}
