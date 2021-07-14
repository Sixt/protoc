package main

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"runtime"
	"testing"
)

func TestExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	if _, exit := execute("true"); exit != 0 {
		t.Fatal(exit)
	}
	if _, exit := execute("false"); exit != 1 {
		t.Fatal(exit)
	}
	if _, exit := execute("sh", "-c", "exit 42"); exit != 42 {
		t.Fatal(exit)
	}
}
func Test_extractBranchMaster(t *testing.T) {
	output := `* remote origin
  Fetch URL: git@github.com:foo/bar.git
  Push  URL: git@github.com:foo/bar.git
  HEAD branch: master
  Remote branch:
    master tracked
  Local branch configured for 'git pull':
    master merges with remote master
  Local ref configured for 'git push':
    master pushes to master (up to date)
`
	branch := extractBranch(output)
	if branch != "master" {
		t.Fatal("failed extracting branch")
	}
}
func Test_extractBranchMain(t *testing.T) {
	output := `* remote origin
  Fetch URL: git@github.com:foo/bar.git
  Push  URL: git@github.com:foo/bar.git
  HEAD branch: main
  Remote branch:
    master tracked
  Local branch configured for 'git pull':
    master merges with remote master
  Local ref configured for 'git push':
    master pushes to master (up to date)
`
	branch := extractBranch(output)
	fmt.Println("branch is ", branch)
	if branch != "main" {
		t.Fatal("failed extracting branch")
	}
}
func Test_extractBranchDefaultMasterBranch(t *testing.T) {
	output := `* remote origin
  Fetch URL: git@github.com:foo/bar.git
  Push  URL: git@github.com:foo/bar.git
  HEAD branch:
  Remote branch:
    master tracked
  Local branch configured for 'git pull':
    master merges with remote master
  Local ref configured for 'git push':
    master pushes to master (up to date)
`
	branch := extractBranch(output)
	if branch != "master" {
		t.Fatal("failed extracting branch")
	}
}

func Test_copyIncludesToCache(t *testing.T) {
	// given
	// check if the includes are present in the project
	checkIfDirIsNotEmpty(t, includesDir)

	tempDir, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	os.Setenv("PROTOC_CACHE_DIR", tempDir)
	defer os.Unsetenv("PROTOC_CACHE_DIR")

	// when
	err = copyIncludesToCache(includesDir)

	// then
	assert.NoError(t, err)
	checkIfDirIsNotEmpty(t, tempDir)
}

func checkIfDirIsNotEmpty(t *testing.T, path string) {
	dirs, err := os.ReadDir(path)
	assert.NoError(t, err)
	for _, d := range dirs {
		f, err := d.Info()
		assert.NoError(t, err)
		assert.True(t, f.Size() != 0)
	}
}
