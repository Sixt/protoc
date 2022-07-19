package main

import (
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
)

var gitAddr = ""

func TestDownload(t *testing.T) {
	os.RemoveAll("testcache")
	os.RemoveAll("testrepo")
	cacheDir = func() string { return "testcache" }

	os.Setenv("GIT_SSL_NO_VERIFY", "true")

	zr, _ := zip.OpenReader("testrepo.zip")
	defer zr.Close()
	for _, f := range zr.File {
		path := filepath.Join(f.Name)
		os.MkdirAll(filepath.Dir(path), 0755)
		src, _ := f.Open()
		if !f.FileInfo().IsDir() {
			dst, _ := os.Create(path)
			io.Copy(dst, src)
			dst.Close()
		}
		src.Close()
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cwd, _ := os.Getwd()
		w.Header().Set("Cache-Control", "no-cache")
		s := strings.SplitN(strings.TrimLeft(r.URL.Path, "/"), "/", 2)
		ep, err := transport.NewEndpoint(path.Join("file://", cwd, s[0]))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ups, err := server.DefaultServer.NewUploadPackSession(ep, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if strings.Contains(r.URL.Path, "info") {
			advs, err := ups.AdvertisedReferences()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			advs.Prefix = [][]byte{
				[]byte("# service=git-upload-pack"),
				[]byte(""),
			}
			w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
			advs.Encode(w)
			return
		}
		defer r.Body.Close()
		var rdr io.ReadCloser = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			rdr, _ = gzip.NewReader(r.Body)
		}
		upakreq := packp.NewUploadPackRequest()
		upakreq.Decode(rdr)
		up, err := ups.UploadPack(r.Context(), upakreq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		up.Encode(w)
	})
	server := httptest.NewUnstartedServer(handler)
	server.StartTLS()
	defer server.Close()

	gitAddr = server.Listener.Addr().String()

	println("running git server: " + gitAddr)

	for _, test := range []struct {
		Path string
		Tag  string
	}{
		{Path: "testrepo/test.proto"},
		{Path: "testrepo/test.proto", Tag: "@latest"},
		{Path: "testrepo/test.proto", Tag: "@v1.0.0"},
		{Path: "testrepo/test.proto", Tag: "@v2.0.0"},
	} {
		if local, err := downloadProto(fmt.Sprintf("%s/%s%s", gitAddr, test.Path, test.Tag)); err != nil {
			t.Error(err)
		} else if local != "testcache/protoc/"+version+"/repos/"+gitAddr+"/"+test.Path {
			t.Fatal(local)
		}
	}
}
