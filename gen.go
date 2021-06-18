// +build generage

package main

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	protoBinariesbaseURL = "https://repo1.maven.org/maven2/com/google/protobuf/protoc"
	protoIncludesBaseUrl = "https://github.com/protocolbuffers/protobuf/releases/download"
	includesDir          = "include"
)

func download(url string) ([]byte, error) {
	fmt.Println(url)
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("USAGE: go run -tags generate gen.go <version>")
	}
	version := os.Args[1]
	//generateProtoBinaries(version)
	generateProtoIncludes(version)
}

func generateProtoIncludes(version string) {
	// any arch for which distribution is packaged can be used. All contain same protos
	arch := "linux-x86_64"
	url := fmt.Sprintf("%[1]s/v%[2]s/protoc-%[2]s-%[3]s.zip", protoIncludesBaseUrl, version, arch)

	body, err := download(url)
	if err != nil {
		log.Fatal(err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		log.Fatal(err)
	}

	ensureIncludesDir()
	for _, zipFile := range zipReader.File {
		if filepath.Ext(zipFile.Name) == ".proto" {
			unzippedFileBytes, err := readZipFile(zipFile)
			if err != nil {
				log.Fatal(err)
			}

			ensureDir(zipFile.Name)
			f, err := os.Create(zipFile.Name)
			if err != nil {
				log.Fatal(err)
			}
			f.Write(unzippedFileBytes)
			f.Close()
		}
	}
}

func ensureDir(path string) {
	dirs := filepath.Dir(path)
	if err := os.MkdirAll(dirs, os.ModePerm); err != nil {
		log.Fatal(err)
	}
}

func ensureIncludesDir() {
	if err := os.RemoveAll(includesDir); err != nil {
		log.Fatal(err)
	}
	ensureDir(includesDir)
}

func readZipFile(zf *zip.File) ([]byte, error) {
	f, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}

func generateProtoBinaries(version string) {
	platforms := map[string]string{
		"linux-x86_32":   "linux_386",
		"linux-x86_64":   "linux_amd64",
		"linux-aarch_64": "linux_arm64",
		"osx-x86_64":     "darwin_amd64",
		"windows-x86_32": "windows_386",
		"windows-x86_64": "windows_amd64",
	}

	for arch, goarch := range platforms {
		url := fmt.Sprintf("%[1]s/%[2]s/protoc-%[2]s-%[3]s.exe", protoBinariesbaseURL, version, arch)
		exe, err := download(url)
		if err != nil {
			log.Fatal(err)
		}

		cksum, err := download(url + ".md5")
		if err != nil {
			log.Fatal(err)
		}

		if s := fmt.Sprintf("%x", md5.Sum(exe)); s != string(cksum) {
			log.Fatalln("checksum mismatch: ", url, s, string(cksum))
		}
		f, err := os.Create(fmt.Sprintf("protoc_exe_%s.go", goarch))
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		fmt.Fprintln(f, "package main")
		fmt.Fprintln(f)
		fmt.Fprint(f, `var protoc = []byte("`)
		defer fmt.Fprintln(f, `")`)
		for _, b := range exe {
			if b == '\n' {
				f.WriteString(`\n`)
				continue
			}
			if b == '\\' {
				f.WriteString(`\\`)
				continue
			}
			if b == '"' {
				f.WriteString(`\"`)
				continue
			}
			if (b >= 32 && b <= 126) || b == '\t' {
				f.Write([]byte{b})
				continue
			}
			fmt.Fprintf(f, "\\x%02x", b)
		}
	}
}
