package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parameter argon2id (minimum rekomendasi OWASP). Bisa diturunkan di tes agar cepat.
var (
	argonMemory  uint32 = 19 * 1024
	argonTime    uint32 = 2
	argonThreads uint8  = 1
)

func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, 32)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var v int
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// PolicyError menjelaskan pelanggaran kebijakan password.
func CheckPolicy(pw string) string {
	if len(pw) < 12 {
		return "Use at least 12 characters."
	}
	var lower, upper, other bool
	for _, r := range pw {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		default:
			other = true
		}
	}
	if !(lower && upper && other) {
		return "Use upper and lower case letters and a digit or symbol."
	}
	return ""
}

// UseFastHashingForTests menurunkan biaya argon2 agar tes cepat. Jangan dipanggil di kode produksi.
func UseFastHashingForTests() {
	if os.Getenv("ORCHIS_ENV") == "production" {
		panic("UseFastHashingForTests tidak boleh dipanggil di produksi")
	}
	argonMemory, argonTime, argonThreads = 64, 1, 1
}
