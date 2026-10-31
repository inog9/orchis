// backup-seal keeps keys out of argv/env and never logs archive contents.
package main

import (
	"flag"
	"fmt"
	"io"
	"orchis-go/internal/backupseal"
	"os"
	"strings"
)

func main() {
	path := flag.String("key-file", "", "private 0600 key file (outside archive)")
	decrypt := flag.Bool("decrypt", false, "authenticate and decrypt to staging stdout")
	flag.Parse()
	f, e := os.Open(*path)
	if e != nil {
		fail()
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 128 {
		fail()
	}
	b, e := io.ReadAll(io.LimitReader(f, 129))
	if e != nil {
		fail()
	}
	key := strings.TrimSpace(string(b))
	if *decrypt {
		e = backupseal.Open(os.Stdout, os.Stdin, key)
	} else {
		e = backupseal.Seal(os.Stdout, os.Stdin, key)
	}
	if e != nil {
		fail()
	}
}
func fail() {
	fmt.Fprintln(os.Stderr, "Backup encryption/authentication failed; no archive is approved for restore.")
	os.Exit(1)
}
