package main

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

//go:generate go run -tags generate gen.go 3.6.1

const version = "3.6.1"

func main() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		dir = os.TempDir()
	}

	protocExeName := fmt.Sprintf("protoc-%s-%s_%s.exe", version, runtime.GOOS, runtime.GOARCH)
	protocExePath := filepath.Join(dir, protocExeName)

	b, err := ioutil.ReadFile(protocExePath)
	if err == nil {
		if md5.Sum(b) != md5.Sum(protoc) {
			// Checksum mismatch, create a new binary in the temporary directory
			protocExePath = filepath.Join(os.TempDir(), protocExeName)
		}
	}

	if err := ioutil.WriteFile(protocExePath, protoc, 0755); err != nil {
		log.Fatal(err)
	}

	cmd := exec.Command(protocExePath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}
	}
}
