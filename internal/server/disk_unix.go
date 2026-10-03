//go:build !windows

package server

import "syscall"

func getDiskSpace(path string) diskSpace {
	var ds diskSpace
	var stat syscall.Statfs_t
	if path != "" && syscall.Statfs(path, &stat) == nil {
		const gb = 1024 * 1024 * 1024
		ds.Total = stat.Blocks * uint64(stat.Bsize) / gb
		ds.Free = stat.Bfree * uint64(stat.Bsize) / gb
		ds.Available = stat.Bavail * uint64(stat.Bsize) / gb
	}
	return ds
}
