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
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

//go:generate go run -tags generate gen.go 3.11.0

// Keep this version in sync with the go:generate statement above
const version = "3.11.0"

type repo interface {
	Checkout(rev string) error
	Fetch() error
}

const latestRev = "latest"

func openRepo(url string) (repo, string, string, error) {
	parts := strings.Split(url, "/")
	for i := len(parts); i > 0; i-- {
		dir := cacheFile("repos", filepath.Join(parts[:i]...))
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
	for _, vcsPath := range []string{
		`^(github\.com/[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+)((/[\p{L}0-9_.\-]+)*)$`,
		`^(bitbucket\.org/[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+)((/[A-Za-z0-9_.\-]+)*)$`,
	} {
		re := regexp.MustCompile(vcsPath)
		if m := re.FindStringSubmatch(url); m != nil {
			if repo, dir, err := tryCloneRepo(m[1]); err == nil {
				return repo, filepath.Join(dir, m[2]), nil
			} else {
				return nil, "", err
			}
		}
	}
	parts := strings.Split(url, "/")
	for i := 1; i <= len(parts); i++ {
		repoURL := path.Join(parts[:i]...)
		if repo, dir, err := tryCloneRepo(repoURL); err == nil {
			return repo, filepath.Join(dir, filepath.Join(parts[i:]...)), nil
		}
	}
	return nil, "", errors.New("clone failed: " + url)
}

func tryCloneRepo(repoURL string) (repo, string, error) {
	dir := cacheFile("repos", repoURL)
	os.MkdirAll(dir, 0755)
	log.Println("Trying to clone", repoURL, "into", dir)
	repo, cloneErr := gitCloneDir(repoURL, dir)
	if cloneErr == nil {
		log.Println("Cloned repository:", dir, repoURL)
		return repo, dir, nil
	}
	if gitInfo, err := os.Stat(filepath.Join(dir, ".git")); err == nil && gitInfo.IsDir() {
		os.RemoveAll(dir)
	}
	return nil, "", cloneErr
}

func downloadProto(url string) (string, error) {
	url = path.Clean(url)
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
	if err != nil {
		if err := repo.Fetch(); err != nil {
			log.Println("fetch failed:", err)
		} else {
			err = repo.Checkout(rev)
		}
	}
	return local, err
}

// processArgs converts protoc command line arguments by replacing remote
// repository URLs with local paths.
func processArgs(in []string) ([]string, []string, error) {
	out := []string{}
	files := []string{}
	for n, arg := range in {
		if arg == "--version" {
			fmt.Println("protoc wrapper " + version)
		}
		if strings.HasPrefix(arg, "-") {
			// Obsolete, but still supported syntax `-I <somedir>`
			// Change to the regular syntax `-I=<somedir>`
			if arg == "-I" && n < len(in)-1 {
				in[n+1] = "-I=" + in[n+1]
				continue
			}
			// Command line options are passed as is, except for remote include paths
			path := ""
			for _, prefix := range []string{"--proto_path=", "-I=", "-I"} {
				if strings.HasPrefix(arg, prefix) {
					path = strings.TrimPrefix(arg, prefix)
					break
				}
			}
			if path != "" {
				if _, err := os.Stat(path); os.IsNotExist(err) {
					arg = "-I=" + cacheFile(filepath.Join("repos", path))
				}
			}
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
// returns command output and exit status (zero on success).
func execute(exe string, args ...string) (string, int) {
	cmd := exec.Command(exe, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				return "", status.ExitStatus()
			}
		}
	}
	output, _ := cmd.Output()
	return string(output), 0
}

// cacheDir returns a path to the local user cache using XDG base directory
// specification or OS standard directories.
var cacheDir = func() string {
	if dir := os.Getenv("PROTOC_CACHE_DIR"); dir != "" {
		return dir
	}
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
func cacheFile(path ...string) string {
	return filepath.Join(append([]string{cacheDir(), "protoc", version}, path...)...)
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
	os.MkdirAll(cacheFile(), 0755)
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
		_, err := execute(protocExePath, args...)
		return err
	}
	for _, f := range files {
		if _, exitCode := execute(protocExePath, append(args, f)...); exitCode != 0 {
			return exitCode
		}
	}
	return 0
}

func main() {
	os.Exit(runProtoc())
}
