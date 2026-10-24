package secrets

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

// newTestHasher uses small parameters: the cost is not what is tested.
func newTestHasher(t *testing.T, memoryMiB, time, maxConcurrent int) *Hasher {
	t.Helper()
	h, err := NewHasher(memoryMiB, time, 1, maxConcurrent)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHasher_HashAndVerify(t *testing.T) {
	ctx := context.Background()
	h := newTestHasher(t, 1, 1, 2)
	authKey := []byte("authKey of 32 bytes, base 64...")

	encoded, err := h.Hash(ctx, authKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=1024,t=1,p=1$") {
		t.Errorf("PHC string %q", encoded)
	}
	if other, _ := h.Hash(ctx, authKey); other == encoded {
		t.Error("two hashes of the same key must have different salts")
	}

	match, rehash, err := h.Verify(ctx, authKey, encoded)
	if err != nil || !match || rehash {
		t.Errorf("right key: match %v, rehash %v, err %v", match, rehash, err)
	}
	match, _, err = h.Verify(ctx, []byte("another key"), encoded)
	if err != nil || match {
		t.Errorf("wrong key: match %v, err %v", match, err)
	}
}

func TestHasher_RehashWithStrongerParameters(t *testing.T) {
	ctx := context.Background()
	weak := newTestHasher(t, 1, 1, 1)
	strong := newTestHasher(t, 2, 2, 1)
	encoded, _ := weak.Hash(ctx, []byte("key"))

	// The stored hash carries its own parameters: it still verifies.
	match, rehash, err := strong.Verify(ctx, []byte("key"), encoded)
	if err != nil || !match || !rehash {
		t.Errorf("old hash: match %v, rehash %v, err %v", match, rehash, err)
	}
}

func TestHasher_MalformedHash(t *testing.T) {
	h := newTestHasher(t, 1, 1, 1)
	for _, encoded := range []string{
		"",
		"$2b$10$bcrypthash",
		"$argon2i$v=19$m=1024,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=16$m=1024,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=0,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=1024,t=1,p=1$not base64!$a2V5",
	} {
		if _, _, err := h.Verify(context.Background(), []byte("key"), encoded); !errors.Is(err, ErrMalformedHash) {
			t.Errorf("Verify(%q): %v, want ErrMalformedHash", encoded, err)
		}
	}
}

func TestHasher_DummyNeverMatches(t *testing.T) {
	h := newTestHasher(t, 1, 1, 1)
	if err := h.VerifyDummy(context.Background(), []byte("anything")); err != nil {
		t.Fatal(err)
	}
}

func TestHasher_ConcurrencyLimit(t *testing.T) {
	h := newTestHasher(t, 1, 1, 1)
	h.slots <- struct{}{} // the only slot is taken

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, []byte("key")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a hash waiting for a slot must give up with its context: %v", err)
	}
	<-h.slots
}

func TestNewHasher_InvalidParameters(t *testing.T) {
	if _, err := NewHasher(0, 1, 1, 1); err == nil {
		t.Error("zero memory must be rejected")
	}
	if _, err := NewHasher(1, 1, 256, 1); err == nil {
		t.Error("more than 255 threads must be rejected")
	}
}

func TestNewToken(t *testing.T) {
	token, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		t.Errorf("token %q: %d bytes, %v", token, len(raw), err)
	}
	sum := sha256.Sum256([]byte(token))
	if !Equal(hash, sum[:]) || !Equal(HashToken(token), hash) {
		t.Error("the hash of a token is the SHA-256 of the token")
	}
	if other, _, _ := NewToken(); other == token {
		t.Error("two tokens must differ")
	}
}

func TestFakeSalt(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	a := FakeSalt(secret, "unknown@example.com")
	if len(a) != 16 {
		t.Fatalf("salt of %d bytes, want 16", len(a))
	}
	if !Equal(a, FakeSalt(secret, "unknown@example.com")) {
		t.Error("the fake salt of an identifier must be stable")
	}
	if Equal(a, FakeSalt(secret, "other@example.com")) {
		t.Error("two identifiers must get different salts")
	}
	if Equal(a, FakeSalt([]byte("another secret"), "unknown@example.com")) {
		t.Error("the salt must depend on the secret of the server")
	}
}

func TestCrockford(t *testing.T) {
	// The recovery key of the vectors of SYNC.md § 3.10: bytes 64 to 83.
	key := make([]byte, 20)
	for i := range key {
		key[i] = byte(64 + i)
	}
	if got := EncodeCrockford(key); got != "810M4GT48N34EJ29995MRKAE9X852MJK" {
		t.Errorf("EncodeCrockford = %q", got)
	}

	tests := []struct {
		in, want string
		ok       bool
	}{
		{"810M-4GT4-8n34", "810M4GT48N34", true},
		{" k7qd 2mxp ", "K7QD2MXP", true},
		{"O0-Il", "0011", true},
		{"K7QU-2MXP", "", false}, // U is not in the alphabet
		{"K7Q*", "", false},
	}
	for _, tt := range tests {
		got, err := NormalizeCrockford(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("NormalizeCrockford(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func TestInvitationCode(t *testing.T) {
	code, err := NewInvitationCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("code %q, want XXXX-XXXX", code)
	}
	hash, err := HashInvitationCode(code)
	if err != nil {
		t.Fatal(err)
	}
	// However it is typed, a code has the same hash.
	typed, _ := HashInvitationCode(strings.ToLower(strings.ReplaceAll(code, "-", " ")))
	if !Equal(hash, typed) {
		t.Error("the hash must not depend on case, spaces or hyphens")
	}
	if _, err := HashInvitationCode("U000-0000"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("invalid code: %v", err)
	}
}
