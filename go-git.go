//+build gogit

package main

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	git "gopkg.in/src-d/go-git.v4"
	plumbing "gopkg.in/src-d/go-git.v4/plumbing"
	transport "gopkg.in/src-d/go-git.v4/plumbing/transport"
	http "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	ssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

func netrcAuth(importPath string) transport.AuthMethod {
	u, err := url.Parse("https://" + importPath)
	if err != nil {
		return nil
	}
	f, err := os.Open(filepath.Join(os.Getenv("HOME"), ".netrc"))
	if err != nil {
		return nil
	}
	defer f.Close()
	username, password, err := netrc(f, u.Host)
	if err != nil {
		return nil
	}
	if username == "" && password == "" {
		return nil
	}
	return &http.BasicAuth{Username: username, Password: password}
}

func sshAuth() transport.AuthMethod {
	auth, _ := ssh.NewSSHAgentAuth("git")
	return auth
}

func auth(url string) (transport.AuthMethod, string) {
	auth := netrcAuth(url)
	schema := "https://"
	if auth == nil {
		auth = sshAuth()
		schema = "ssh://"
	}
	return auth, schema
}

type gitRepo struct {
	url  string
	repo *git.Repository
}

func gitOpenDir(url, dir string) (repo, error) {
	r, err := git.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	return &gitRepo{url: url, repo: r}, nil
}

func gitCloneDir(url, dir string) (repo, error) {
	auth, schema := auth(url)
	r, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:  schema + url + ".git",
		Auth: auth,
	})
	if err != nil {
		return nil, err
	}
	return &gitRepo{url: url, repo: r}, nil
}

func (r *gitRepo) Checkout(rev string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return err
	}
	if rev == "" || rev == latestRev {
		ref, err := r.repo.Head()
		if err != nil {
			return err
		}
		rev = ref.Hash().String()
		log.Println("Using HEAD revision", rev)
	} else {
		tagrefs, err := r.repo.Tags()
		if err != nil {
			return err
		}
		found := false
		tagrefs.ForEach(func(t *plumbing.Reference) error {
			if !found && strings.TrimPrefix(t.Name().String(), "refs/tags/") == rev {
				found = true
				rev = t.Hash().String()
				annotated, err := r.repo.TagObject(t.Hash())
				if err == nil {
					rev = annotated.Target.String()
				}
				log.Println("Using tag ", t.Name().String(), "revision", rev)
			}
			return nil
		})
	}

	err = w.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(rev),
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *gitRepo) Fetch() error {
	w, err := r.repo.Worktree()
	if err != nil {
		return err
	}
	auth, _ := auth(r.url)
	if err := w.Pull(&git.PullOptions{
		RemoteName: "origin",
		Auth:       auth,
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		// Ignore if pull fails, try our best to work offline
		return err
	}
	return nil
}
