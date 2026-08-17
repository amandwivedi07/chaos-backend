// Package utils provides small, dependency-light helpers used across modules.
package utils

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

// HashPassword hashes a plaintext password with bcrypt.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	return string(b), err
}

// CheckPassword reports whether plain matches the bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
