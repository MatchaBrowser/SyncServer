// Package testclient implements the cryptography of the client (SYNC.md § 3),
// independently of the server code: the end-to-end tests use it to talk to
// the server as Matcha does, and TestSpecVectors checks it against § 3.10.
package testclient

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

const (
	// RecordBlock, BlobBlock and AvatarBlock are the padding blocks of a
	// record, of an extension package and of the avatar of the account
	// (§ 3.5).
	RecordBlock = 256
	BlobBlock   = 64 * 1024
	AvatarBlock = 4 * 1024

	formatVersion = 0x01
	nonceBytes    = 12
)

// PasswordKeys are derived from the master password (§ 3.2).
type PasswordKeys struct {
	Master []byte
	Auth   []byte
	Wrap   []byte
}

// DerivePasswordKeys runs PBKDF2-HMAC-SHA256 on the password, normalized in
// NFKD, then HKDF for authKey and wrapKey.
func DerivePasswordKeys(password string, salt []byte, iterations int) (PasswordKeys, error) {
	master, err := pbkdf2.Key(sha256.New, norm.NFKD.String(password), salt, iterations, 32)
	if err != nil {
		return PasswordKeys{}, err
	}
	return PasswordKeys{
		Master: master,
		Auth:   subkey(master, "matcha.auth.v1"),
		Wrap:   subkey(master, "matcha.wrap.v1"),
	}, nil
}

// AccountKeys are derived from the account key.
type AccountKeys struct {
	Data []byte
	ID   []byte
}

func DeriveAccountKeys(accountKey []byte) AccountKeys {
	return AccountKeys{
		Data: subkey(accountKey, "matcha.data.v1"),
		ID:   subkey(accountKey, "matcha.id.v1"),
	}
}

// RecoveryKeys are derived from the recovery key.
type RecoveryKeys struct {
	Auth []byte
	Wrap []byte
}

func DeriveRecoveryKeys(recoveryKey []byte) RecoveryKeys {
	return RecoveryKeys{
		Auth: subkey(recoveryKey, "matcha.recovery-auth.v1"),
		Wrap: subkey(recoveryKey, "matcha.recovery-wrap.v1"),
	}
}

// subkey is HKDF-SHA256 with an empty salt and a 32-byte output.
func subkey(key []byte, info string) []byte {
	out, err := hkdf.Key(sha256.New, key, nil, info, 32)
	if err != nil {
		panic(err) // only fails for an output longer than 255 hashes
	}
	return out
}

// Seal is AES-256-GCM: nonce ‖ ciphertext ‖ tag.
func Seal(key, nonce, plaintext, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != nonceBytes {
		return nil, fmt.Errorf("nonce of %d bytes", len(nonce))
	}
	return gcm.Seal(bytes.Clone(nonce), nonce, plaintext, aad), nil
}

// Open reverses Seal.
func Open(key, sealed, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < nonceBytes+gcm.Overhead() {
		return nil, errors.New("sealed data too short")
	}
	return gcm.Open(nil, sealed[:nonceBytes], sealed[nonceBytes:], aad)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Pad completes data to a multiple of block with 0x80 then 0x00
// (ISO/IEC 7816-4).
func Pad(data []byte, block int) []byte {
	padded := append(bytes.Clone(data), 0x80)
	for len(padded)%block != 0 {
		padded = append(padded, 0x00)
	}
	return padded
}

// Unpad reverses Pad.
func Unpad(padded []byte) ([]byte, error) {
	i := bytes.LastIndexFunc(padded, func(r rune) bool { return r != 0 })
	if i < 0 || padded[i] != 0x80 {
		return nil, errors.New("invalid padding")
	}
	return padded[:i], nil
}

// AccountKeyAAD is the associated data of both envelopes of the account key.
const AccountKeyAAD = "matcha.account-key.v1"

// WrapAccountKey seals the account key with wrapKey or recoveryWrapKey.
func WrapAccountKey(wrapKey, nonce, accountKey []byte) ([]byte, error) {
	return Seal(wrapKey, nonce, accountKey, []byte(AccountKeyAAD))
}

func recordAAD(id string, keyVersion int) []byte {
	return []byte(fmt.Sprintf("matcha.record.v1|%s|%d", strings.ToLower(id), keyVersion))
}

// EncryptRecord returns the payload of a record: base 64 of
// 0x01 ‖ Seal(dataKey, padded content, aad).
func EncryptRecord(dataKey []byte, id string, keyVersion int, content, nonce []byte) (string, error) {
	sealed, err := Seal(dataKey, nonce, Pad(content, RecordBlock), recordAAD(id, keyVersion))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(append([]byte{formatVersion}, sealed...)), nil
}

// DecryptRecord reverses EncryptRecord.
func DecryptRecord(dataKey []byte, id string, keyVersion int, payload string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || raw[0] != formatVersion {
		return nil, errors.New("unknown format version")
	}
	padded, err := Open(dataKey, raw[1:], recordAAD(id, keyVersion))
	if err != nil {
		return nil, err
	}
	return Unpad(padded)
}

// EncryptBlob returns an encrypted extension package:
// 0x01 ‖ Seal(blobKey, padded package, "matcha.blob.v1|<blobId>").
func EncryptBlob(blobKey []byte, blobID string, pkg, nonce []byte) ([]byte, error) {
	aad := []byte("matcha.blob.v1|" + strings.ToLower(blobID))
	sealed, err := Seal(blobKey, nonce, Pad(pkg, BlobBlock), aad)
	if err != nil {
		return nil, err
	}
	return append([]byte{formatVersion}, sealed...), nil
}

func avatarAAD(accountID string, keyVersion int) []byte {
	return []byte(fmt.Sprintf("matcha.avatar.v1|%s|%d", strings.ToLower(accountID), keyVersion))
}

// EncryptAvatar returns the encrypted avatar of an account:
// 0x01 ‖ Seal(dataKey, padded picture, aad).
func EncryptAvatar(dataKey []byte, accountID string, keyVersion int, picture, nonce []byte) ([]byte, error) {
	sealed, err := Seal(dataKey, nonce, Pad(picture, AvatarBlock), avatarAAD(accountID, keyVersion))
	if err != nil {
		return nil, err
	}
	return append([]byte{formatVersion}, sealed...), nil
}

// DecryptAvatar reverses EncryptAvatar.
func DecryptAvatar(dataKey []byte, accountID string, keyVersion int, encrypted []byte) ([]byte, error) {
	if len(encrypted) == 0 || encrypted[0] != formatVersion {
		return nil, errors.New("unknown format version")
	}
	padded, err := Open(dataKey, encrypted[1:], avatarAAD(accountID, keyVersion))
	if err != nil {
		return nil, err
	}
	return Unpad(padded)
}

// DerivedID is the identifier of a record named by a key (§ 2.2): the first
// 16 bytes of HMAC-SHA256(idKey, name), as a version 8 UUID.
func DerivedID(idKey []byte, name string) string {
	mac := hmac.New(sha256.New, idKey)
	mac.Write([]byte(name))
	b := mac.Sum(nil)[:16]
	b[6] = b[6]&0x0F | 0x80
	b[8] = b[8]&0x3F | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// DisplayRecoveryKey shows the 20 bytes of a recovery key in base 32 of
// Crockford, 8 groups of 4 (§ 3.2).
func DisplayRecoveryKey(key []byte) (string, error) {
	if len(key) != 20 {
		return "", fmt.Errorf("recovery key of %d bytes", len(key))
	}
	var chars []byte
	var buffer uint64
	bits := 0
	for _, c := range key {
		buffer = buffer<<8 | uint64(c)
		bits += 8
		for bits >= 5 {
			bits -= 5
			chars = append(chars, crockford[(buffer>>bits)&31])
		}
	}
	var groups []string
	for i := 0; i < len(chars); i += 4 {
		groups = append(groups, string(chars[i:i+4]))
	}
	return strings.Join(groups, "-"), nil
}
