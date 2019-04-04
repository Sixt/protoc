// +build windows

package main

import "os"

// Windows implementation of lock files can be probably borrowed here:
// https://github.com/gofrs/flock/blob/master/flock_winapi.go

func lock(f *os.File) error   { return nil }
func unlock(f *os.File) error { return nil }
