package testclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// The test vectors of SYNC.md § 3.10.

// span returns the bytes from..to inclusive: "octets 0 à 15" is 0x00..0x0F.
func span(from, to int) []byte {
	b := make([]byte, 0, to-from+1)
	for i := from; i <= to; i++ {
		b = append(b, byte(i))
	}
	return b
}

func hexEqual(t *testing.T, name string, got []byte, want string) {
	t.Helper()
	if hex.EncodeToString(got) != want {
		t.Errorf("%s = %x, want %s", name, got, want)
	}
}

func TestSpecVectors(t *testing.T) {
	const password = "Café crème, 2026 !"
	salt := span(0, 15)
	accountKey := span(32, 63)
	recoveryKey := span(64, 83)

	keys, err := DerivePasswordKeys(password, salt, 600000)
	if err != nil {
		t.Fatal(err)
	}
	hexEqual(t, "masterKey", keys.Master, "1b17deb72bf81bc32f13fa4e0ce6e2458325b151098172bf5f60579d1725872a")
	hexEqual(t, "authKey", keys.Auth, "5181174923363be5cfc1ddd47761224fe7653b627bb8a16b9b56e44cbf3bb477")
	hexEqual(t, "wrapKey", keys.Wrap, "ef4e8ca94c79a8831d0be6956954257ea820aae32daad637623c33d573978bbd")

	account := DeriveAccountKeys(accountKey)
	hexEqual(t, "dataKey", account.Data, "681021aae32d4b8896879d3b68426f203ffe80a3505f4f88688e393b71c8da7c")
	hexEqual(t, "idKey", account.ID, "1566d83727cc0f51bd6130bcfbeb63b5edeed74d0aa752c98507d8610a539dac")

	recovery := DeriveRecoveryKeys(recoveryKey)
	hexEqual(t, "recoveryAuthKey", recovery.Auth, "e09957c068a62d3d182a3eb20567f6081ae9d5ad89034125ffc2376aad9b2ee0")
	hexEqual(t, "recoveryWrapKey", recovery.Wrap, "c99ca23f11c93fad7ae8bfc65d45937f1d3ba0bbb9f47770d68dccc3c41ee275")

	wrapped, err := WrapAccountKey(keys.Wrap, span(100, 111), accountKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.StdEncoding.EncodeToString(wrapped); got != "ZGVmZ2hpamtsbW5vtSkcc3+EmMJgIoX82mvpDSrkO645VA8Tv2uuthfFNFVN2Rq2ZgR73kssLbiYmJgk" {
		t.Errorf("wrappedAccountKey = %s", got)
	}
	recoveryWrapped, err := WrapAccountKey(recovery.Wrap, span(112, 123), accountKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.StdEncoding.EncodeToString(recoveryWrapped); got != "cHFyc3R1dnd4eXp7s5N8UZFUOtzOSzjCwNalJVB4uWQ2w4/Y3OMaYjcFGDaG8ywQwJ9tnr7Xz4peNUdl" {
		t.Errorf("recoveryWrappedAccountKey = %s", got)
	}
	if opened, err := Open(keys.Wrap, wrapped, []byte(AccountKeyAAD)); err != nil || !bytes.Equal(opened, accountKey) {
		t.Errorf("opening wrappedAccountKey: %v", err)
	}

	display, err := DisplayRecoveryKey(recoveryKey)
	if err != nil || display != "810M-4GT4-8N34-EJ29-995M-RKAE-9X85-2MJK" {
		t.Errorf("recovery key displayed %q, %v", display, err)
	}
}

func TestSpecVectors_DerivedIDs(t *testing.T) {
	idKey := DeriveAccountKeys(span(32, 63)).ID
	vectors := map[string]string{
		"setting:searchEngine": "29C6E709-25C1-837D-A81F-9938153247BB",
		"site-zoom:google.com": "D841E19B-C45E-8758-A7F8-EE09968642E7",
		"site-permission:9b1e4d2a-0c3f-4a6b-8e5d-7f2c1a9b3e40|https://meet.google.com|camera": "3574A498-BBBE-89DD-97E5-E8B259C7B0BF",
		"extension:0123456789abcdef0123456789abcdef":                                          "B146F347-53E0-8D14-B690-A7E7477085F7",
		"extension-app:com.example.DarkReader.Extension":                                      "B57FF5FB-0A29-8DBD-A38F-18FFB8646F9E",
		"default-profile": "F0C1AD76-39FD-8912-B97C-7D330800F8F3",
		"default-space":   "8467CEA7-A22F-842E-AF9E-B52B799680AE",
	}
	for name, want := range vectors {
		if got := DerivedID(idKey, name); got != want {
			t.Errorf("DerivedID(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestSpecVectors_Record(t *testing.T) {
	dataKey := DeriveAccountKeys(span(32, 63)).Data
	const id = "7C4B2E9A-1D3F-4B6E-8A0C-5E2D9F1B3A76"
	content := []byte(`{"deleted":false,"fields":{"name":"Personnel","order":0},"modifiedAt":"2026-10-17T08:30:12.345Z","modifiedBy":"3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10","schema":1,"type":"profile"}`)
	if len(content) != 177 {
		t.Fatalf("content of %d bytes, want 177", len(content))
	}
	const want = "AXx9fn+AgYKDhIWGhy4GlMRzc+YJeTnjZErem3VDPTwk4WRTSxyMJDg2dKkUu4uAoKOZt3k+uh7IcGj4HGY2C4227yL/t64q" +
		"qc7gcBc4I8Nl/V1fUWESR9zeh72w6IDlRn8EZRMvtIEJCzt7F59kiu0eaWbgjf4rFD76riMiHHt/87v3+llWYu1G0aOXCi+/" +
		"smVS94aTfQOi+AmaSeEP+nb3zuE6/IfBEG6SrlO4rHaAljgF50eyLRxUKdDRS6bER/9J48TEGAtO5IZxFYAUESyey9ikCWO3" +
		"hnFvY/0XIyfSmOZ4aoeZmy3jPGSW6LeHhrsoAc/XdAXlRa8H6KRE2DBHSTqF0rVRlvVOweBtG/adxKH7kcFuGRDgEA1p"

	payload, err := EncryptRecord(dataKey, id, 1, content, span(124, 135))
	if err != nil {
		t.Fatal(err)
	}
	if payload != want {
		t.Errorf("payload = %s", payload)
	}
	if raw, _ := base64.StdEncoding.DecodeString(payload); len(raw) != 285 {
		t.Errorf("payload of %d bytes, want 285", len(raw))
	}

	decrypted, err := DecryptRecord(dataKey, id, 1, payload)
	if err != nil || !bytes.Equal(decrypted, content) {
		t.Errorf("decrypting the record: %v", err)
	}
	// A content copied under another identifier does not open.
	if _, err := DecryptRecord(dataKey, "E1A0F6B2-0000-4000-8000-000000000000", 1, payload); err == nil {
		t.Error("a record must not open under another identifier")
	}
}

func TestSpecVectors_Blob(t *testing.T) {
	pkg := []byte{0x50, 0x4B, 0x03, 0x04}
	for range 4 {
		pkg = append(pkg, span(0, 255)...)
	}
	if sum := sha256.Sum256(pkg); hex.EncodeToString(sum[:]) != "55cf14b0da2830b4a0c792188f89ecb6d7331b8260a9af1b6fc5a65abf78adcc" {
		t.Errorf("package SHA-256 = %x", sum)
	}

	encrypted, err := EncryptBlob(span(150, 181), "5A1C9E3B-7D2F-4B8A-9C6E-1F0D3B7A2E58", pkg, span(136, 147))
	if err != nil {
		t.Fatal(err)
	}
	if len(encrypted) != 65565 {
		t.Errorf("encrypted package of %d bytes, want 65565", len(encrypted))
	}
	if sum := sha256.Sum256(encrypted); hex.EncodeToString(sum[:]) != "21c61a10b905ebc08287797aac0bf30e30134f9f21af66e71ccb185e492f4a85" {
		t.Errorf("encrypted package SHA-256 = %x", sum)
	}
}

func TestPad(t *testing.T) {
	for _, n := range []int{0, 1, 255, 256, 300} {
		data := bytes.Repeat([]byte{0xAB}, n)
		padded := Pad(data, RecordBlock)
		if len(padded)%RecordBlock != 0 || len(padded) <= n {
			t.Errorf("Pad(%d bytes) = %d bytes", n, len(padded))
		}
		if got, err := Unpad(padded); err != nil || !bytes.Equal(got, data) {
			t.Errorf("Unpad(Pad(%d bytes)): %v", n, err)
		}
	}
}
