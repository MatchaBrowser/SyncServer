package testclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"sort"
	"strings"
)

// The cryptography of the vault (VAULT.md § 2 to § 6), independent of the
// Swift client: TestVaultVectors checks it against § 12.

// VaultRootKeyAAD is the associated data of both envelopes of the vault root
// key (VAULT.md § 2.2).
const VaultRootKeyAAD = "matcha.vault-root-key.v1"

// Lengths of the details and of a file chunk (VAULT.md § 3.2, § 6).
const (
	DetailsFloor   = 1024
	FileChunkBytes = 16 * 1024 * 1024
)

// WrapVaultRootKey seals the vault root key with wrapKey or recoveryWrapKey.
func WrapVaultRootKey(wrapKey, nonce, vaultRootKey []byte) ([]byte, error) {
	return Seal(wrapKey, nonce, vaultRootKey, []byte(VaultRootKeyAAD))
}

// VaultKeys are the keys of the personal vault, derived from the vault root
// key (VAULT.md § 2.1, § 2.3).
type VaultKeys struct {
	Vault    []byte
	ItemWrap []byte
	Manifest []byte
}

func DeriveVaultKeys(vaultRootKey []byte) VaultKeys {
	vault := subkey(vaultRootKey, "matcha.vault.personal.v1")
	return VaultKeys{Vault: vault, ItemWrap: subkey(vault, "matcha.vault.item-wrap.v1"), Manifest: subkey(vault, "matcha.vault.manifest.v1")}
}

// VaultID is the identifier of the personal vault: the first 16 bytes of
// HKDF(vaultKey, "matcha.vault.id.v1"), as a version 8 UUID (VAULT.md § 2.3).
func VaultID(keys VaultKeys) string {
	return uuid8(subkey(keys.Vault, "matcha.vault.id.v1"))
}

// ManifestID and BucketID are the identifiers of the records of the manifest
// (VAULT.md § 4.1).

func ManifestID(idKey []byte, vaultID string) string {
	return DerivedID(idKey, "vault-manifest:"+strings.ToLower(vaultID))
}

func BucketID(idKey []byte, vaultID, bucket string) string {
	return DerivedID(idKey, "vault-bucket:"+strings.ToLower(vaultID)+"|"+bucket)
}

// BucketOf is the bucket of an item: the first hexadecimal character of its
// identifier, in lowercase.
func BucketOf(itemID string) string { return strings.ToLower(itemID)[:1] }

// Padme is the padded length of Padmé (VAULT.md § 3.2).
func Padme(length int) int {
	if length < 2 {
		return length
	}
	e := bits.Len(uint(length)) - 1
	s := bits.Len(uint(e))
	mask := 1<<(e-s) - 1
	return (length + mask) &^ mask
}

// PadDetails completes the details to Padmé(max(n + 1, 1024)) bytes.
func PadDetails(data []byte) []byte {
	target := Padme(max(len(data)+1, DetailsFloor))
	padded := append(bytes.Clone(data), 0x80)
	return append(padded, make([]byte, target-len(padded))...)
}

func itemAAD(part, vaultID, itemID string, keyVersion int) []byte {
	return []byte(fmt.Sprintf("matcha.vault.%s.v1|%s|%s|%d", part, strings.ToLower(vaultID), strings.ToLower(itemID), keyVersion))
}

// ItemFields are the three sealed parts of an item, as the fields of its
// record carry them in base 64 (VAULT.md § 3.1).
type ItemFields struct {
	ItemKey  []byte
	Overview []byte
	Details  []byte
}

// ItemNonces are the nonces of the three parts; tests draw them, or fix them
// for the vectors.
type ItemNonces struct{ ItemKey, Overview, Details []byte }

// SealItem seals an item: its key under itemWrapKey, its overview and details
// under its key.
func SealItem(keys VaultKeys, vaultID, itemID string, keyVersion int, itemKey, overview, details []byte, nonces ItemNonces) (ItemFields, error) {
	var fields ItemFields
	var err error
	if fields.ItemKey, err = sealVersioned(keys.ItemWrap, nonces.ItemKey, itemKey, itemAAD("item-key", vaultID, itemID, keyVersion)); err != nil {
		return fields, err
	}
	if fields.Overview, err = sealVersioned(itemKey, nonces.Overview, Pad(overview, RecordBlock), itemAAD("overview", vaultID, itemID, keyVersion)); err != nil {
		return fields, err
	}
	fields.Details, err = sealVersioned(itemKey, nonces.Details, PadDetails(details), itemAAD("details", vaultID, itemID, keyVersion))
	return fields, err
}

// OpenItem reverses SealItem, and returns the item key, the overview and the
// details.
func OpenItem(keys VaultKeys, vaultID, itemID string, keyVersion int, fields ItemFields) (itemKey, overview, details []byte, err error) {
	if itemKey, err = openVersioned(keys.ItemWrap, fields.ItemKey, itemAAD("item-key", vaultID, itemID, keyVersion)); err != nil {
		return nil, nil, nil, fmt.Errorf("item key: %w", err)
	}
	padded, err := openVersioned(itemKey, fields.Overview, itemAAD("overview", vaultID, itemID, keyVersion))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("overview: %w", err)
	}
	if overview, err = Unpad(padded); err != nil {
		return nil, nil, nil, err
	}
	if padded, err = openVersioned(itemKey, fields.Details, itemAAD("details", vaultID, itemID, keyVersion)); err != nil {
		return nil, nil, nil, fmt.Errorf("details: %w", err)
	}
	details, err = Unpad(padded)
	return itemKey, overview, details, err
}

// Digest is the digest of a version of an item (VAULT.md § 3.3).
func (f ItemFields) Digest() string {
	h := sha256.New()
	h.Write([]byte("matcha.vault.item.v1"))
	for _, part := range [][]byte{f.ItemKey, f.Overview, f.Details} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(part)))
		h.Write(length[:])
		h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Bucket is a bucket of the manifest, once opened: item identifier in
// lowercase → digest, and the tombstones of the items deleted, identifier →
// date of the deletion. The fields are in the order of their names, as the
// JSON of the vectors.
type Bucket struct {
	Deleted map[string]string `json:"deleted"`
	Items   map[string]string `json:"items"`
}

// Root is the root of the manifest, once opened. Clock is its version
// vector: device identifier in lowercase → number of its writes.
type Root struct {
	Buckets    map[string]string `json:"buckets"`
	Clock      map[string]int64  `json:"clock"`
	ModifiedAt string            `json:"modifiedAt"`
	ModifiedBy string            `json:"modifiedBy"`
}

// Succeeds tells whether r succeeds other: every entry of the clock of other
// is at most that of r (VAULT.md § 4.1).
func (r Root) Succeeds(other Root) bool {
	for device, n := range other.Clock {
		if r.Clock[device] < n {
			return false
		}
	}
	return true
}

// SameClock tells whether both clocks have the same entries.
func (r Root) SameClock(other Root) bool {
	return r.Succeeds(other) && other.Succeeds(r)
}

func bucketAAD(vaultID, bucket string, keyVersion int) []byte {
	return []byte(fmt.Sprintf("matcha.vault.bucket.v1|%s|%s|%d", strings.ToLower(vaultID), bucket, keyVersion))
}

func rootAAD(vaultID string, keyVersion int) []byte {
	return []byte(fmt.Sprintf("matcha.vault.manifest.v1|%s|%d", strings.ToLower(vaultID), keyVersion))
}

// SealBucket returns the manifest field of a bucket record.
func SealBucket(keys VaultKeys, vaultID, bucket string, keyVersion int, content Bucket, nonce []byte) ([]byte, error) {
	if content.Items == nil {
		content.Items = map[string]string{}
	}
	if content.Deleted == nil {
		content.Deleted = map[string]string{}
	}
	plain, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	return sealVersioned(keys.Manifest, nonce, Pad(plain, RecordBlock), bucketAAD(vaultID, bucket, keyVersion))
}

// OpenBucket reverses SealBucket.
func OpenBucket(keys VaultKeys, vaultID, bucket string, keyVersion int, sealed []byte) (Bucket, error) {
	var content Bucket
	padded, err := openVersioned(keys.Manifest, sealed, bucketAAD(vaultID, bucket, keyVersion))
	if err != nil {
		return content, err
	}
	plain, err := Unpad(padded)
	if err != nil {
		return content, err
	}
	err = json.Unmarshal(plain, &content)
	return content, err
}

// SealRoot returns the manifest field of the root record.
func SealRoot(keys VaultKeys, vaultID string, keyVersion int, content Root, nonce []byte) ([]byte, error) {
	if content.Buckets == nil {
		content.Buckets = map[string]string{}
	}
	if content.Clock == nil {
		content.Clock = map[string]int64{}
	}
	plain, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	return sealVersioned(keys.Manifest, nonce, Pad(plain, RecordBlock), rootAAD(vaultID, keyVersion))
}

// OpenRoot reverses SealRoot.
func OpenRoot(keys VaultKeys, vaultID string, keyVersion int, sealed []byte) (Root, error) {
	var content Root
	padded, err := openVersioned(keys.Manifest, sealed, rootAAD(vaultID, keyVersion))
	if err != nil {
		return content, err
	}
	plain, err := Unpad(padded)
	if err != nil {
		return content, err
	}
	err = json.Unmarshal(plain, &content)
	return content, err
}

// BucketDigest is the digest of a sealed bucket that the root cites.
func BucketDigest(sealed []byte) string {
	sum := sha256.Sum256(sealed)
	return hex.EncodeToString(sum[:])
}

// EncryptFileChunk returns chunk index of n of a file:
// 0x01 ‖ Seal(fileKey, chunk padded to 64 KiB, aad) (VAULT.md § 6).
func EncryptFileChunk(fileKey []byte, fileID string, index, count int, chunk, nonce []byte) ([]byte, error) {
	aad := fileChunkAAD(fileID, index, count)
	return sealVersioned(fileKey, nonce, Pad(chunk, BlobBlock), aad)
}

// DecryptFileChunk reverses EncryptFileChunk.
func DecryptFileChunk(fileKey []byte, fileID string, index, count int, encrypted []byte) ([]byte, error) {
	padded, err := openVersioned(fileKey, encrypted, fileChunkAAD(fileID, index, count))
	if err != nil {
		return nil, err
	}
	return Unpad(padded)
}

func fileChunkAAD(fileID string, index, count int) []byte {
	return []byte(fmt.Sprintf("matcha.vault.file.v1|%s|%d|%d", strings.ToLower(fileID), index, count))
}

func sealVersioned(key, nonce, plaintext, aad []byte) ([]byte, error) {
	sealed, err := Seal(key, nonce, plaintext, aad)
	if err != nil {
		return nil, err
	}
	return append([]byte{formatVersion}, sealed...), nil
}

func openVersioned(key, sealed, aad []byte) ([]byte, error) {
	if len(sealed) == 0 || sealed[0] != formatVersion {
		return nil, errors.New("unknown format version")
	}
	return Open(key, sealed[1:], aad)
}

// VaultState is what a Mac knows of the vault: the root in force, and the
// items it cites with their digest. It is the verified state of VAULT.md
// § 5.2, reduced to what the end-to-end tests need.
type VaultState struct {
	Root  Root
	Items map[string]string
}

// Verification is the result of comparing what the server shows to a
// verified state (VAULT.md § 5.2, § 5.3).
type Verification struct {
	// Accepted are the items whose record matches the root.
	Accepted map[string]string
	// Waiting are the items the root cites without a matching record.
	Waiting []string
	// Hidden are the items of the verified state the root no longer cites,
	// without a tombstone: a divergence.
	Hidden []string
	// Deleted are the items of the verified state that a tombstone removes.
	Deleted []string
	// Uncited are the item records the root does not cite.
	Uncited []string
	// Divergent: the root does not succeed the root in force.
	Divergent bool
}

// ServerVault is the vault as the server shows it: the opened root and
// buckets, and the digest of each item record (empty for a deletion).
type ServerVault struct {
	Root    Root
	Buckets map[string]Bucket
	Records map[string]string
}

// Verify compares the server's vault to a verified state. A state without a
// root (a new Mac) accepts any root.
func Verify(server ServerVault, state VaultState) Verification {
	v := Verification{Accepted: map[string]string{}, Divergent: !server.Root.Succeeds(state.Root)}
	cited, tombstones := map[string]string{}, map[string]bool{}
	for _, bucket := range server.Buckets {
		for id, digest := range bucket.Items {
			cited[id] = digest
		}
		for id := range bucket.Deleted {
			tombstones[id] = true
		}
	}
	for id, digest := range cited {
		if server.Records[id] == digest {
			v.Accepted[id] = digest
		} else {
			v.Waiting = append(v.Waiting, id)
		}
	}
	for id := range state.Items {
		if _, ok := cited[id]; ok {
			continue
		}
		if tombstones[id] {
			v.Deleted = append(v.Deleted, id)
		} else {
			v.Hidden = append(v.Hidden, id)
		}
	}
	for id, digest := range server.Records {
		if _, ok := cited[id]; !ok && digest != "" {
			v.Uncited = append(v.Uncited, id)
		}
	}
	v.Divergent = v.Divergent || len(v.Hidden) > 0
	for _, list := range [][]string{v.Waiting, v.Hidden, v.Deleted, v.Uncited} {
		sort.Strings(list)
	}
	return v
}
