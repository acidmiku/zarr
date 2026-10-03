package server

import (
	"syscall"
	"unsafe"
)

func getDiskSpace(path string) diskSpace {
	var ds diskSpace
	if path == "" {
		return ds
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ds
	}
	var available, total, free uint64
	fn := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
	ok, _, _ := fn.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if ok != 0 {
		const gb = 1024 * 1024 * 1024
		ds.Total = total / gb
		ds.Free = free / gb
		ds.Available = available / gb
	}
	return ds
}
