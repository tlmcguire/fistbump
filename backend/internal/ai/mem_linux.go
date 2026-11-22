package ai

import "golang.org/x/sys/unix"

// TotalRAM returns the computer's total memory in bytes, or 0 if it cannot be read.
func TotalRAM() uint64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return uint64(info.Totalram) * uint64(info.Unit)
}
