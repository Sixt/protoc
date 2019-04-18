package main

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

//go:generate go run -tags generate gen.go 3.7.1

const version = "3.7.1"

type repo interface {
	Checkout(rev string) error
}

const latestRev = "latest"

func openRepo(url string) (repo, string, string, error) {
	parts := strings.Split(url, "/")
	for i := len(parts); i > 0; i-- {
		dir := filepath.Join(cacheDir(), "protoc", "repos", filepath.Join(parts[:i]...))
		// Sometimes go-git gives false positives, check for .git directory before PlainOpen()
		if info, err := os.Stat(filepath.Join(dir, ".git")); err != nil || !info.IsDir() {
			continue
		}
		repoURL := path.Join(parts[:i]...)
		repo, err := gitOpenDir(repoURL, dir)
		if err == nil {
			log.Println("Use cached repository:", dir)
			return repo, dir, filepath.Join(dir, filepath.Join(parts[i:]...)), nil
		}
	}
	return nil, "", "", errors.New("failed to open " + url)
}

func cloneRepo(url string) (repo, string, error) {
	parts := strings.Split(url, "/")
	for i := 1; i <= len(parts); i++ {
		repoURL := path.Join(parts[:i]...)
		dir := filepath.Join(cacheDir(), "protoc", "repos", repoURL)
		os.MkdirAll(dir, 0755)
		log.Println("Trying to clone", repoURL, "into", dir)
		repo, err := gitCloneDir(repoURL, dir)
		if err == nil {
			log.Println("Cloned repository:", dir, repoURL)
			return repo, filepath.Join(dir, filepath.Join(parts[i:]...)), nil
		} else if gitInfo, err := os.Stat(filepath.Join(dir, ".git")); err == nil && gitInfo.IsDir() {
			os.RemoveAll(dir)
		}
	}
	return nil, "", errors.New("clone failed: " + url)
}

func downloadProto(url string) (string, error) {
	url = filepath.Clean(url)
	rev := ""
	if i := strings.LastIndex(url, "@"); i >= 0 {
		rev = url[i+1:]
		url = url[:i]
	}
	repo, dir, local, err := openRepo(url)
	if err == nil && rev == latestRev {
		log.Println("Invalidate cached directory:", dir)
		os.RemoveAll(dir)
	}
	if err != nil || rev == latestRev {
		repo, local, err = cloneRepo(url)
	}
	if err != nil {
		return "", err
	}
	err = repo.Checkout(rev)
	return local, err
}

// processArgs converts protoc command line arguments by replacing remote
// repository URLs with local paths.
func processArgs(in []string) ([]string, []string, error) {
	out := []string{}
	files := []string{}
	for _, arg := range in {
		if strings.HasPrefix(arg, "-") {
			// Command line options are passed as is
			out = append(out, arg)
		} else if _, err := os.Stat(arg); !os.IsNotExist(err) {
			// Local proto files are passed as is. Stat() errors are ignored allowing
			// protoc to handle it.
			files = append(files, arg)
		} else {
			// Remote proto files are downloaded
			local, err := downloadProto(arg)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, "-I"+filepath.Dir(local))
			files = append(files, local)
		}
	}
	return out, files, nil
}

func expandDirs(dirs []string) []string {
	files := []string{}
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if filepath.Ext(path) == ".proto" {
					files = append(files, path)
				}
				return nil
			})
		} else {
			files = append(files, dir)
		}
	}
	return files
}

// execute runs a command with the provided arguments, using current stdio, and
// returns command exit status (zero on success).
func execute(exe string, args ...string) int {
	cmd := exec.Command(exe, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus()
			}
		}
	}
	return 0
}

// cacheDir returns a path to the local user cache using XDG base directory
// specification or OS standard directories.
var cacheDir = func() string {
	switch runtime.GOOS {
	case "darwin":
		return os.Getenv("HOME") + "/Library/Caches"
	case "windows":
		return os.Getenv("LOCALAPPDATA")
	case "linux":
		if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
			return dir
		}
		return filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return ""
}

// cacheFile returns a path to the local user cache file inside the protoc
// cache directory.
func cacheFile(path string) string {
	return filepath.Join(cacheDir(), "protoc", path)
}

// extractProtoc extracts real protoc binary for the current platform. Returns
// absolute path to the protoc binary, or an error.
func extractProtoc() (string, error) {
	protocExeName := fmt.Sprintf("protoc-%s-%s_%s.exe", version, runtime.GOOS, runtime.GOARCH)
	protocExePath := cacheFile(protocExeName)
	b, err := ioutil.ReadFile(protocExePath)
	if err != nil || md5.Sum(b) != md5.Sum(protoc) {
		// Checksum mismatch, create a new binary in the temporary directory
		err = ioutil.WriteFile(protocExePath, protoc, 0755)
	}
	return protocExePath, err
}

// runProtoc() is the main function. It is moved outside of main to make use of
// defer statemenets. All that main() does now is os.Exit() which is not
// defer-friendly at all.
func runProtoc() int {
	os.MkdirAll(filepath.Join(cacheDir(), "protoc"), 0755)
	lockFile, err := os.Create(cacheFile("protoc.lock"))
	if err != nil {
		log.Fatal(err)
	}
	defer lockFile.Close()

	if err := lock(lockFile); err != nil {
		log.Fatal(err)
	}
	defer unlock(lockFile)

	protocExePath, err := extractProtoc()
	if err != nil {
		log.Fatal("extract protoc:", err)
	}

	args, files, err := processArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	files = expandDirs(files)
	if len(files) == 0 {
		return execute(protocExePath, args...)
	}
	for _, f := range files {
		if exitCode := execute(protocExePath, append(args, f)...); exitCode != 0 {
			return exitCode
		}
	}
	return 0
}

func main() {
	os.Exit(runProtoc())
}
