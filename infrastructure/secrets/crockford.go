package secrets

import (
	"crypto/sha256"
	"errors"
	"strings"
)

// crockfordAlphabet is the base 32 of Crockford: no I, L, O or U.
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ErrInvalidCode is returned for a code with a character outside the
// alphabet of Crockford.
var ErrInvalidCode = errors.New("invalid character in code")

// EncodeCrockford encodes b in base 32 of Crockford, most significant bits
// first, without padding.
func EncodeCrockford(b []byte) string {
	var out strings.Builder
	var buffer uint32
	bits := 0
	for _, c := range b {
		buffer = buffer<<8 | uint32(c)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out.WriteByte(crockfordAlphabet[(buffer>>bits)&31])
		}
	}
	if bits > 0 {
		out.WriteByte(crockfordAlphabet[(buffer<<(5-bits))&31])
	}
	return out.String()
}

// NormalizeCrockford reads a code as typed by a person: case, spaces and
// hyphens are ignored, O reads 0, I and L read 1. Any other character outside
// the alphabet (U included) makes the code invalid (SYNC.md § 3.2).
func NormalizeCrockford(code string) (string, error) {
	var out strings.Builder
	for _, r := range strings.ToUpper(code) {
		switch r {
		case ' ', '-':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		if !strings.ContainsRune(crockfordAlphabet, r) {
			return "", ErrInvalidCode
		}
		out.WriteRune(r)
	}
	return out.String(), nil
}

// NewInvitationCode returns an invitation code of 8 characters in 2 groups
// of 4, like K7QD-2MXP (40 bits of entropy).
func NewInvitationCode() (string, error) {
	random, err := RandomBytes(5)
	if err != nil {
		return "", err
	}
	code := EncodeCrockford(random)
	return code[:4] + "-" + code[4:], nil
}

// HashInvitationCode returns the hash under which an invitation is stored.
// The code is normalized first, so that it matches however it was typed.
func HashInvitationCode(code string) ([]byte, error) {
	normalized, err := NormalizeCrockford(code)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(normalized))
	return sum[:], nil
}
