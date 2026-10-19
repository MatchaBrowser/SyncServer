// Package secrets holds the cryptography of the server: Argon2id hashes of the
// keys sent by the clients, opaque tokens, fake salts of prelogin and codes in
// Crockford base 32 (SYNC.md § 4.9).
package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2SaltBytes = 16
	argon2KeyBytes  = 32
)

// ErrMalformedHash is returned for a stored hash that is not an Argon2id PHC
// string.
var ErrMalformedHash = errors.New("malformed argon2id hash")

// Argon2Params are the cost parameters of Argon2id.
type Argon2Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
}

// Hasher hashes and verifies authKey and recoveryAuthKey with Argon2id. The
// hashes are PHC strings, which carry their parameters: they can be raised,
// and a hash made with weaker ones is redone at the next login.
//
// Each computation takes MemoryKiB of memory: at most maxConcurrent run at
// the same time, the others wait.
type Hasher struct {
	params Argon2Params
	slots  chan struct{}
	dummy  string
}

// NewHasher returns a Hasher. memoryMiB is in MiB.
func NewHasher(memoryMiB, time, threads, maxConcurrent int) (*Hasher, error) {
	if memoryMiB < 1 || time < 1 || threads < 1 || threads > 255 || maxConcurrent < 1 {
		return nil, fmt.Errorf("invalid argon2id parameters: memory %d MiB, time %d, threads %d, concurrency %d", memoryMiB, time, threads, maxConcurrent)
	}
	h := &Hasher{
		params: Argon2Params{MemoryKiB: uint32(memoryMiB) * 1024, Time: uint32(time), Threads: uint8(threads)},
		slots:  make(chan struct{}, maxConcurrent),
	}
	// The hash checked for unknown accounts, so that they answer in the same
	// time as the known ones.
	random, err := RandomBytes(argon2KeyBytes)
	if err != nil {
		return nil, err
	}
	if h.dummy, err = h.Hash(context.Background(), random); err != nil {
		return nil, err
	}
	return h, nil
}

// Hash returns the PHC string of secret, with a random salt.
func (h *Hasher) Hash(ctx context.Context, secret []byte) (string, error) {
	salt, err := RandomBytes(argon2SaltBytes)
	if err != nil {
		return "", err
	}
	key, err := h.compute(ctx, secret, salt, h.params)
	if err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.params.MemoryKiB, h.params.Time, h.params.Threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify reports whether secret matches the PHC string encoded, and whether
// that hash was made with weaker parameters than the current ones (it should
// then be replaced by Hash).
func (h *Hasher) Verify(ctx context.Context, secret []byte, encoded string) (match, rehash bool, err error) {
	params, salt, key, err := parsePHC(encoded)
	if err != nil {
		return false, false, err
	}
	computed, err := h.compute(ctx, secret, salt, params)
	if err != nil {
		return false, false, err
	}
	if !Equal(computed, key) {
		return false, false, nil
	}
	rehash = params.MemoryKiB < h.params.MemoryKiB || params.Time < h.params.Time || params.Threads < h.params.Threads
	return true, rehash, nil
}

// VerifyDummy spends the time of a verification for an account that does not
// exist. It never matches.
func (h *Hasher) VerifyDummy(ctx context.Context, secret []byte) error {
	_, _, err := h.Verify(ctx, secret, h.dummy)
	return err
}

func (h *Hasher) compute(ctx context.Context, secret, salt []byte, params Argon2Params) ([]byte, error) {
	select {
	case h.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-h.slots }()
	return argon2.IDKey(secret, salt, params.Time, params.MemoryKiB, params.Threads, argon2KeyBytes), nil
}

// parsePHC reads "$argon2id$v=19$m=65536,t=3,p=4$<salt>$<key>".
func parsePHC(encoded string) (Argon2Params, []byte, []byte, error) {
	var params Argon2Params
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return params, nil, nil, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return params, nil, nil, ErrMalformedHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.MemoryKiB, &params.Time, &params.Threads); err != nil {
		return params, nil, nil, ErrMalformedHash
	}
	if params.MemoryKiB == 0 || params.Time == 0 || params.Threads == 0 {
		return params, nil, nil, ErrMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return params, nil, nil, ErrMalformedHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return params, nil, nil, ErrMalformedHash
	}
	return params, salt, key, nil
}
