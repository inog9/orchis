// Package envfile loads deployment secrets without putting them in container arguments.
package envfile

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

// Load reads a bounded, private JSON object selected by the operator. Values from
// the file override the environment; only application variables are accepted.
func Load() error {
	path := os.Getenv("ORCHIS_ENV_FILE")
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("cannot open application secret file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 || info.Mode().Perm()&0077 != 0 {
		return errors.New("application secret file must be a private regular file (0600)")
	}
	dec := json.NewDecoder(io.LimitReader(f, 65537))
	var values map[string]string
	if err = dec.Decode(&values); err != nil || values == nil {
		return errors.New("invalid application secret file")
	}
	var trailing any
	if dec.Decode(&trailing) != io.EOF {
		return errors.New("trailing data in application secret file")
	}
	for key, value := range values {
		if !strings.HasPrefix(key, "ORCHIS_") || key == "ORCHIS_ENV_FILE" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return errors.New("invalid variable in application secret file")
		}
	}
	for key, value := range values {
		if err = os.Setenv(key, value); err != nil {
			return errors.New("cannot load application secret file")
		}
	}
	return nil
}
