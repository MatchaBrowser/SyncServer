package secrets

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// tokenBytes is the entropy of an access, refresh or recovery token.
const tokenBytes = 32

// RandomBytes returns n bytes from the system random generator.
func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// NewToken returns an opaque token for the client and its hash, the only
// thing the server keeps (SYNC.md § 4.9).
func NewToken() (token string, hash []byte, err error) {
	random, err := RandomBytes(tokenBytes)
	if err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(random)
	return token, HashToken(token), nil
}

// HashToken returns the SHA-256 of a token, under which it is stored.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// FakeSalt is the salt answered by prelogin for an unknown identifier:
// HMAC-SHA256(secret of the server, identifier), truncated to 16 bytes. It is
// stable, so that nobody can tell which accounts exist (SYNC.md § 4.3).
func FakeSalt(secret []byte, email string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(email))
	return mac.Sum(nil)[:16]
}

// Equal compares two byte strings in constant time.
func Equal(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
