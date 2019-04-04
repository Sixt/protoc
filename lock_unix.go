// +build darwin dragonfly freebsd linux netbsd openbsd

package main

import (
	"os"
	"syscall"
)

func doLock(f *os.File, lockType int) (err error) {
	for {
		err = syscall.Flock(int(f.Fd()), lockType)
		if err != syscall.EINTR {
			break
		}
	}
	// Ignore errors if locks are not supported
	if err == syscall.ENOSYS || err == syscall.ENOTSUP || err == syscall.EOPNOTSUPP {
		err = nil
	}
	return err
}

func lock(f *os.File) error   { return doLock(f, syscall.LOCK_EX) }
func unlock(f *os.File) error { return doLock(f, syscall.LOCK_UN) }
