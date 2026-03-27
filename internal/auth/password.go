package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword creates a bcrypt hash of the password with the default cost factor.
// bcrypt automatically generates and embeds a random salt, making rainbow tables infeasible.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// VerifyPassword checks if the provided password matches the stored bcrypt hash.
// Uses bcrypt's constant-time comparison to prevent timing attacks.
func VerifyPassword(password, storedHash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)) == nil
}
