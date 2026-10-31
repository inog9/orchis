// Package backupseal streams the standard age format with a separate random
// 256-bit key. Callers must authenticate the complete archive before restoring.
package backupseal

import (
	"encoding/hex"
	"errors"
	"filippo.io/age"
	"io"
)

const workFactor = 16 // 64 MiB; key is random 256-bit, never a human password.
func validate(key string) error {
	b, e := hex.DecodeString(key)
	if e != nil || len(b) != 32 {
		return errors.New("backup key must be 32 random bytes in hex")
	}
	return nil
}
func Seal(dst io.Writer, src io.Reader, key string) error {
	if e := validate(key); e != nil {
		return e
	}
	r, e := age.NewScryptRecipient(key)
	if e != nil {
		return e
	}
	r.SetWorkFactor(workFactor)
	w, e := age.Encrypt(dst, r)
	if e != nil {
		return e
	}
	if _, e = io.Copy(w, src); e != nil {
		return e
	}
	return w.Close()
}
func Open(dst io.Writer, src io.Reader, key string) error {
	if e := validate(key); e != nil {
		return e
	}
	identity, e := age.NewScryptIdentity(key)
	if e != nil {
		return e
	}
	identity.SetMaxWorkFactor(workFactor)
	r, e := age.Decrypt(src, identity)
	if e != nil {
		return e
	}
	_, e = io.Copy(dst, r)
	return e
}
