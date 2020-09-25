//+build !gogit

package main

import (
	"fmt"
	"regexp"
)

type gitRepo struct {
	url string
	dir string
}

var re = regexp.MustCompile(`.*HEAD branch: (.*)\n`) // regex to extract default branch name of a repo

func gitCmd(args ...string) (string, error) {
	output, code := execute("git", args...)
	if code != 0 {
		return "", fmt.Errorf("git failed: exit code %d", code)
	}
	return output, nil
}

func gitOpenDir(url, dir string) (repo, error) {
	_, err := gitCmd("-C", dir, "rev-parse")
	return &gitRepo{url: url, dir: dir}, err
}

func gitCloneDir(url, dir string) (repo, error) {
	_, err := gitCmd("clone", "https://"+url, dir)
	return &gitRepo{url: url, dir: dir}, err
}

func (r *gitRepo) Checkout(rev string) error {
	output, err := gitCmd("-C", r.dir, "remote", "show", "origin")
	if err != nil {
		return err
	}
	defaultBranch := extractBranch(output)
	if _, err := gitCmd("-C", r.dir, "checkout", defaultBranch); err != nil {
		return err
	}
	if rev == "" || rev == latestRev {
		rev = "HEAD"
	}
	_, err = gitCmd("-C", r.dir, "checkout", "-q", rev)
	return err
}

func (r *gitRepo) Fetch() error {
	_, err := gitCmd("-C", r.dir, "pull")
	return err
}

func extractBranch(output string) string {
	if !re.MatchString(output) {
		return "master"
	}
	return re.FindStringSubmatch(output)[1]
}
