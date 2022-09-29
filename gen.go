//go:build generate

package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	protoIncludesBaseUrl = "https://github.com/protocolbuffers/protobuf/releases/download"
	includesDir          = "include"
)

func download(url string) ([]byte, error) {
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
