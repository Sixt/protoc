package main

import (
	"bytes"
	"testing"
)

func TestNetRc(t *testing.T) {
	user, pass, err := netrc(bytes.NewBufferString(`
	machine foo.com password 1234 login FOO
	# machine example.com login WRONG password WRONGPASS
	#
	# machine

	machine example.com
		login
			john
		password j0hnd0e

	machine bar.com login BAR password BARBAR
	`), "example.com")

	if user != "john" || pass != "j0hnd0e" || err != nil {
		t.Fatal(user, pass, err)
	}
}
