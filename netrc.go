package main

import (
	"bufio"
	"bytes"
	"io"
)

func netrc(r io.Reader, machine string) (username, password string, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Split(func(b []byte, eof bool) (int, []byte, error) {
		skip := 0
		for {
			n, w, err := bufio.ScanWords(b, eof)
			if err != nil || len(w) == 0 || w[0] != '#' {
				return skip + n, w, err
			}
			i := bytes.IndexByte(b[n:], '\n')
			if i < 0 {
				return len(b), nil, nil
			}
			skip = skip + n + i
			b = b[n+i:]
		}
	})
	foundMachine := false
	for scanner.Scan() {
		if foundMachine {
			switch scanner.Text() {
			case "login":
				if scanner.Scan() {
					username = scanner.Text()
				}
			case "password":
				if scanner.Scan() {
					password = scanner.Text()
				}
			}
		}
		switch scanner.Text() {
		case "machine":
			foundMachine = false
			if scanner.Scan() && scanner.Text() == machine {
				foundMachine = true
			}
		case "defult":
			foundMachine = true
		}
	}
	return username, password, scanner.Err()
}
