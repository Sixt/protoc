package main

import (
	"bytes"
	"crypto/md5"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

//go:generate go run -tags generate gen.go 22.2

// Keep this version in sync with the go:generate statement above
const (
	version                     = "3.22.2"
	protoBinariesBaseURL        = "https://repo1.maven.org/maven2/com/google/protobuf/protoc"
	includesDir                 = "include"
	includesCacheFilePermission = 0664
	includesCacheDirPermission  = 0775
)

var platforms = map[string]string{
	"linux_386":     "linux-x86_32",
	"linux_amd64":   "linux-x86_64",
	"linux_arm64":   "linux-aarch_64",
	"darwin_amd64":  "osx-x86_64",
	"darwin_arm64":  "osx-aarch_64",
	"windows_386":   "windows-x86_32",
	"windows_amd64": "windows-x86_64",
}

//go:embed include/google/protobuf
var include embed.FS

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
		if err = repo.Fetch(); err != nil {
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
	var out []string
	var files []string
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
	//copy include files to cache
	err := copyIncludesToCache(includesDir)
	if err != nil {
		return nil, nil, err
	}
	out = append(out, "-I="+cacheFile(filepath.Join(includesDir)))
	return out, files, nil
}

// copies the upstream proto includes to the cache.
// Does not copy if the file is already present.
func copyIncludesToCache(dirPath string) error {
	dst := filepath.Join(cacheDir(), "protoc", version, dirPath)

	err := os.MkdirAll(dst, includesCacheDirPermission)
	if err != nil {
		log.Fatal(err)
	}
	dirs, err := include.ReadDir(dirPath)

	if err != nil {
		log.Fatal(err)
	}
	for _, fd := range dirs {

		if fd.IsDir() {
			if err := copyIncludesToCache(path.Join(dirPath, fd.Name())); err != nil {
				log.Fatal(err)
			}
			continue
		}

		srcFB, err := fs.ReadFile(include, path.Join(dirPath, fd.Name()))
		if err != nil {
			log.Fatal(err)
		}
		dstfp := path.Join(dst, fd.Name())

		if _, err = os.Stat(dstfp); err != nil && os.IsNotExist(err) {
			if err = ioutil.WriteFile(dstfp, srcFB, includesCacheFilePermission); err != nil {
				log.Fatal(err)
			}
		} else if err != nil {
			log.Fatal(err)
		}
	}

	return err
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
	var stdoutBuf bytes.Buffer
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				return "", status.ExitStatus()
			}
		}
	}
	output := string(stdoutBuf.Bytes())
	return output, 0
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

// downloadProtoc downloads protoc binary for the current platform. Returns
// absolute path to the protoc binary, or an error
func downloadProtoc() (string, error) {
	var arch string
	var ok bool
	runtimeArch := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	if arch, ok = platforms[runtimeArch]; !ok {
		return "", fmt.Errorf("unable to resolve architecture for GOOS=%s GOARCH=%s", runtime.GOOS, runtime.GOARCH)
	}

	protocExeName := fmt.Sprintf("protoc-%s-%s_%s.exe", version, runtime.GOOS, runtime.GOARCH)
	protocExePath := cacheFile(protocExeName)

	if _, err := os.Stat(protocExePath); err == nil {
		return protocExePath, nil
	}

	var err error
	defer func() {
		if err != nil {
			// if we have download or checksum validation error need to clean up created file
			// os.PathError let us know that temp empty file wasn't created by os.Create(), e.g. permission issue
			if _, ok := err.(*os.PathError); !ok {
				if rerr := os.Remove(protocExePath); rerr != nil {
					fmt.Println("unable to delete file %w", rerr)
				}
			}
		}
	}()

	log.Println("saving protoc to path: ", protocExePath)
	url := fmt.Sprintf("%[1]s/%[2]s/protoc-%[2]s-%[3]s.exe", protoBinariesBaseURL, version, arch)

	err = downloadFile(protocExePath, url)
	if err != nil {
		return "", err
	}
	err = os.Chmod(protocExePath, 0755)
	if err != nil {
		return "", err
	}
	cksum, err := download(url + ".md5")
	if err != nil {
		return "", err
	}
	f, err := os.Open(protocExePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	if s := fmt.Sprintf("%x", h.Sum(nil)); s != string(cksum) {
		err := fmt.Errorf("checksum mismatch: %s, %s, %s", url, s, string(cksum))
		return "", err
	}

	return protocExePath, nil
}

// runProtoc() is the main function. It is moved outside of main to make use of
// defer statements. All that main() does now is os.Exit() which is not
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

	protocExePath, err := downloadProtoc()
	if err != nil {
		log.Fatal("download protoc:", err)
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

func download(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}

// downloadFile downloads and saves file
func downloadFile(filepath string, url string) (err error) {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	os.Exit(runProtoc())
}
