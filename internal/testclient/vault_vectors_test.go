package testclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// The test vectors of VAULT.md § 12. The keys of SYNC.md § 3.10 are taken as
// they are given there (TestSpecVectors checks them).

func fill(b byte, n int) []byte { return bytes.Repeat([]byte{b}, n) }

func fromHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func base64Equal(t *testing.T, name string, got []byte, want string) {
	t.Helper()
	if g := base64.StdEncoding.EncodeToString(got); g != want {
		t.Errorf("%s = %s, want %s", name, g, want)
	}
}

const (
	vectorWrapKey         = "61399e6410861ea6bd2318e9e8b5d39d5027c31e523594d6646f8d7bc8743319"
	vectorRecoveryWrapKey = "c99ca23f11c93fad7ae8bfc65d45937f1d3ba0bbb9f47770d68dccc3c41ee275"
	vectorIDKey           = "1566d83727cc0f51bd6130bcfbeb63b5edeed74d0aa752c98507d8610a539dac"
	vectorItemID          = "0F8C2A6E-5B3D-4E7A-9C1F-2D6B8E4A7C30"
	vectorOverview        = `{"category":"login","favorite":false,"match":"site","modifiedAt":"2026-11-08T09:15:00.000Z","modifiedBy":"3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10","subtitle":"jeremy","tags":["dev"],"title":"GitHub","trashedAt":null,"urls":["https://github.com/login"]}`
	vectorDetails         = `{"createdAt":"2026-11-08T09:15:00.000Z","credentials":[{"id":"C1","password":"correct horse battery staple","type":"basic-auth","username":"jeremy"},{"algorithm":"sha1","digits":6,"id":"C2","issuer":"GitHub","period":30,"secret":"JBSWY3DPEHPK3PXP","type":"totp"}],"fields":[],"files":[],"history":[],"notes":""}`
	vectorOverviewField   = "AaSkpKSkpKSkpKSkpJTImjZEfuEDI9mXbunaMltEPJY2/aq0qzHS6ZE5EpB5Q2wq82wtAGXM4xTXcvHg9l07GEsCt6sC8BXFwvN4tSZeeDuBoLQ8fvPnM1Tc/UTmgA6nfkatLW3hLhkMFt/XDgGRLqBrIYKcoHf6kst5tAm6nuaXUKBpr3Q8Z9LyYT9JeHjx9cvdR3YemdhgLjARp3tOF2wYW/uu0y+lJE7c7qPdE4tKt2wY2lCMvSOd8U71b/KS1+akOg/MIJ+gmJk07Qfo6fdN1pXZ/AovOL4AEgzh9ihxLcS9Nnmb7RZpCmGwWr6GYn1gXXDTiQ0HiqgZJdOtwQZqOmYssXhYg+LMl7L3N/o6U3GsFjmumIOrTxaA"
	vectorBucketField     = "Aaampqampqampqampv20zjnk5BEolTd57Py/IIEQJqx0M3d5dmamU4DybL4A5lCeqpXZJONfTELyEBiWAkBn9VQwbt/kYEeA6Jc0lwcOO/eSWe2ZI0JunA5BatHVlllBgCc/UVpnH20pgxICeSRNLNCq9ECAromaQzHzF9wE2+a8Rh0BmDwy/eVyYaiZNLgtlh7cFmFITBUr3nT5/D6bljDXEvrHEigAJOBpo/UqbnXkK1fFrLeRZNpGV/Kw2wIATMOGTWt9/Z5KnxydIhHnzkooGSGteNLxZDTRSIndyAHcQS5jwxvnQaigJcm7BeSmbB1IxzW5umpQEv37TEbKDYYi8OGa8QNY6inHbli208UZUFkxmhelm/NDWh9j"
	vectorRootField       = "Aaenp6enp6enp6enpzr7aWaj5an1/5ZxtXyOkuFsZC0f4+urNSyZxd3QrkD8f401pDR8ZBdIP7KNV5H5LP+0LH2Zs4EdKz9UZl/t1liEzGSJG8Y3FBj2//d/K+/awVYJmLRyFwVD0MRhtidF7oF7EpDnr1Yfuo5HmveAqU4TgyDz0Z60uCkvdbvrQsaXK5DAqDUBc/to98qdN2aC8+eUj8Jmh0yietWJtCH+UfYWPQBHcWtC+vosjvYL7OD7FGYJKbPAZIHUvHx/QYPV16yPLVs3oIR6kzYzC3xVO8zk9QjDfAezGTLZFAJEZj5aJ3kEQf4Q604UBLNmOE0rvUOeHGnqrnQdGKx+Rn2X4XZSftBt0+GEYJkx1oWGsw8f"
)

func TestVaultVectors(t *testing.T) {
	vaultRootKey := span(208, 239)

	wrapped, err := WrapVaultRootKey(fromHex(t, vectorWrapKey), fill(0xA1, 12), vaultRootKey)
	if err != nil {
		t.Fatal(err)
	}
	base64Equal(t, "wrappedVaultRootKey", wrapped, "oaGhoaGhoaGhoaGhEOxauvt9iwApCVrUiHK62nCeXEyW8TvEbI8xQnr0mK1RKoY25LICmBnLMoj46sw+")
	recoveryWrapped, err := WrapVaultRootKey(fromHex(t, vectorRecoveryWrapKey), fill(0xA2, 12), vaultRootKey)
	if err != nil {
		t.Fatal(err)
	}
	base64Equal(t, "recoveryWrappedVaultRootKey", recoveryWrapped, "oqKioqKioqKioqKi/O/doW8QrILBadMk0oZpdE7H+MfGYVN1ydlMb/xNojP0DQpiSgDsZGGd4pyRvBoi")
	if _, err := Open(fromHex(t, vectorWrapKey), wrapped, []byte(AccountKeyAAD)); err == nil {
		t.Error("the envelope of the vault root key must not open as one of the account key")
	}

	keys := DeriveVaultKeys(vaultRootKey)
	hexEqual(t, "vaultKey", keys.Vault, "8261db72fb9ebf224e624f04aa9d445a64aaac04ac53f8e9668f3c67e93dd6de")
	hexEqual(t, "itemWrapKey", keys.ItemWrap, "a66570248ec1e58cf77e60053367b6a8163f92d4960ad6aa18e5f3baf2fffbce")
	hexEqual(t, "manifestKey", keys.Manifest, "2358b6d07fdec265ec600aff1e6a4923395e279705b34b348ece7c92a9dd69af")

	idKey := fromHex(t, vectorIDKey)
	vaultID := VaultID(keys)
	for name, got := range map[string][2]string{
		"vaultId":  {vaultID, "02AFD736-3BA5-8284-BE72-68DCE1F17671"},
		"root":     {ManifestID(idKey, vaultID), "12FFDC1F-CB5A-851F-A0E8-FF1D5482C62D"},
		"bucket 0": {BucketID(idKey, vaultID, "0"), "F16781CF-1918-8D31-9C78-8D9F12E85B96"},
	} {
		if got[0] != got[1] {
			t.Errorf("%s = %s, want %s", name, got[0], got[1])
		}
	}

	if got := string(itemAAD("item-key", vaultID, vectorItemID, 1)); got != "matcha.vault.item-key.v1|02afd736-3ba5-8284-be72-68dce1f17671|0f8c2a6e-5b3d-4e7a-9c1f-2d6b8e4a7c30|1" {
		t.Errorf("aad of itemKey = %s", got)
	}
	nonces := ItemNonces{ItemKey: fill(0xA3, 12), Overview: fill(0xA4, 12), Details: fill(0xA5, 12)}
	item, err := SealItem(keys, vaultID, vectorItemID, 1, fill(0x11, 32), []byte(vectorOverview), []byte(vectorDetails), nonces)
	if err != nil {
		t.Fatal(err)
	}
	base64Equal(t, "itemKey", item.ItemKey, "AaOjo6Ojo6Ojo6Ojo5B41srfBJztsNM+eH4hy0i/PDDMhFs/U9zzWqtC7AOpTsibBHGeJtXBEqCrGI9kCQ==")
	base64Equal(t, "overview", item.Overview, vectorOverviewField)
	if len(item.Details) != 1053 {
		t.Errorf("details of %d bytes, want 1053", len(item.Details))
	}
	sum := sha256.Sum256(item.Details)
	hexEqual(t, "SHA-256 of details", sum[:], "dd6959378ff61c404dc6a03ea42870e400f294607ca0876abffaae8d1d56c555")
	if digest := item.Digest(); digest != "cf16a0a026abc5e2824ef98804918c1ed1951800859bbc7a1d835978dc240665" {
		t.Errorf("item digest = %s", digest)
	}

	_, overview, details, err := OpenItem(keys, vaultID, vectorItemID, 1, item)
	if err != nil || string(overview) != vectorOverview || string(details) != vectorDetails {
		t.Errorf("open item: %v\n%s\n%s", err, overview, details)
	}
	// A part of another item, or another part, does not open.
	other := "1F8C2A6E-5B3D-4E7A-9C1F-2D6B8E4A7C30"
	if _, _, _, err := OpenItem(keys, vaultID, other, 1, item); err == nil {
		t.Error("an item copied under another identifier must not open")
	}
	swapped := item
	swapped.Overview, swapped.Details = item.Details, item.Overview
	if _, _, _, err := OpenItem(keys, vaultID, vectorItemID, 1, swapped); err == nil {
		t.Error("overview and details swapped must not open")
	}

	bucket, err := SealBucket(keys, vaultID, "0", 1, Bucket{
		Deleted: map[string]string{"5c1d9e7a-2b4f-4a6c-8e0d-1f3b5a7c9e2d": "2026-11-01T10:00:00.000Z"},
		Items:   map[string]string{strings.ToLower(vectorItemID): item.Digest()},
	}, fill(0xA6, 12))
	if err != nil {
		t.Fatal(err)
	}
	base64Equal(t, "bucket", bucket, vectorBucketField)
	if d := BucketDigest(bucket); d != "68af6b3fc517a14fb9b0ec35e8407651e3cc513d97f064ec9441af472ab9fafc" {
		t.Errorf("bucket digest = %s", d)
	}
	root := Root{
		Buckets: map[string]string{"0": BucketDigest(bucket)}, Clock: map[string]int64{"3f2a8c1e-6b0d-4c5e-9f1a-2b7d8e4c6a10": 1},
		ModifiedAt: "2026-11-08T09:15:00.000Z", ModifiedBy: "3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10",
	}
	sealedRoot, err := SealRoot(keys, vaultID, 1, root, fill(0xA7, 12))
	if err != nil {
		t.Fatal(err)
	}
	base64Equal(t, "root", sealedRoot, vectorRootField)
	if opened, err := OpenRoot(keys, vaultID, 1, sealedRoot); err != nil || opened.Clock["3f2a8c1e-6b0d-4c5e-9f1a-2b7d8e4c6a10"] != 1 || opened.Buckets["0"] != BucketDigest(bucket) {
		t.Errorf("open root: %+v, %v", opened, err)
	}
	if _, err := OpenBucket(keys, vaultID, "1", 1, bucket); err == nil {
		t.Error("bucket 0 must not open as bucket 1")
	}
	if _, err := OpenRoot(keys, vaultID, 1, bucket); err == nil {
		t.Error("a bucket must not open as the root")
	}

	for n, want := range map[int]int{0: 1024, 1023: 1024, 1024: 1088, 1500: 1536, 5000: 5120, 20000: 20480, 70000: 71680} {
		if got := len(PadDetails(make([]byte, n))); got != want {
			t.Errorf("details of %d bytes padded to %d, want %d", n, got, want)
		}
	}

	content := bytes.Repeat([]byte("Matcha vault file\n"), 100)
	hexSum := sha256.Sum256(content)
	hexEqual(t, "SHA-256 of the file", hexSum[:], "5b499b55ca0b253aab15898936a55e6d66fd062726a394455c682c803d768d43")
	const fileID = "3B9E1C4A-7D2F-4E8B-A6C0-5F1D2E3A4B59"
	chunk, err := EncryptFileChunk(fill(0x22, 32), fileID, 0, 1, content, fill(0xA8, 12))
	if err != nil {
		t.Fatal(err)
	}
	chunkSum := sha256.Sum256(chunk)
	if len(chunk) != 65565 {
		t.Errorf("chunk of %d bytes, want 65565", len(chunk))
	}
	hexEqual(t, "SHA-256 of the chunk", chunkSum[:], "6bcc43e4c2969c2db20d701bd625b3637ac1debd9483c8ee44007d5d5641296d")
	if _, err := DecryptFileChunk(fill(0x22, 32), fileID, 0, 2, chunk); err == nil {
		t.Error("a chunk must not open with another count of chunks")
	}
}
