package ai

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// memoryStatusEx is MEMORYSTATUSEX from the Windows API.
// https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/ns-sysinfoapi-memorystatusex
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// TotalRAM returns the computer's total memory in bytes, or 0 if it cannot be read.
func TotalRAM() uint64 {
	var s memoryStatusEx
	s.Length = uint32(unsafe.Sizeof(s))
	if r, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&s))); r == 0 {
		return 0
	}
	return s.TotalPhys
}
