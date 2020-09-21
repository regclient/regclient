// +build !windows

package regclient

import (
	"os"
	"syscall"
)

func getFileOwner(stat os.FileInfo) (int, int, error) {
	var uid, gid int
	if sysstat, ok := stat.Sys().(*syscall.Stat_t); ok {
		uid = int(sysstat.Uid)
		gid = int(sysstat.Gid)
	}
	return uid, gid, nil
}
