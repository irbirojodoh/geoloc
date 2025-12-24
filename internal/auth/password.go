package auth

import (
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/sha3"
)

// HashPassword creates a two-chain hash: SHA3-512 -> SHA-256
// h1 = SHA3-512(password)
// h2 = SHA-256(h1) <- stored in database
func HashPassword(password string) string {
	// First hash: SHA3-512
	h1 := sha3.Sum512([]byte(password))

	// Second hash: SHA-256
	h2 := sha256.Sum256(h1[:])

	// Return hex-encoded string
	return hex.EncodeToString(h2[:])
}

// VerifyPassword checks if the provided password matches the stored hash
func VerifyPassword(password, storedHash string) bool {
	computedHash := HashPassword(password)
	return computedHash == storedHash
}
