package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// RandomToken returns a cryptographically random hex string of 2n chars.
// Used for refresh tokens, email verification and password reset tokens.
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SHA256Hex returns the hex sha256 of s. Opaque tokens are stored hashed so a
// database leak does not leak usable tokens.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
