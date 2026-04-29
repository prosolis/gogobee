package crypto

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func testKey() []byte {
	// 32 bytes for AES-256
	return []byte("01234567890123456789012345678901")
}

// ── Encrypt / Decrypt Round-Trip ───────────────────────────────────────────

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := testKey()
	plaintext := []byte("hello, arena champion")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("round-trip failed: got %q, want %q", got, plaintext)
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	key := testKey()
	ciphertext, err := Encrypt(key, []byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	got, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty plaintext, got %d bytes", len(got))
	}
}

func TestEncrypt_DifferentNonces(t *testing.T) {
	key := testKey()
	plaintext := []byte("same input")

	ct1, _ := Encrypt(key, plaintext)
	ct2, _ := Encrypt(key, plaintext)

	if bytes.Equal(ct1, ct2) {
		t.Error("two encryptions of the same plaintext should produce different ciphertexts (random nonces)")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key := testKey()
	plaintext := []byte("secret data")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	wrongKey := []byte("99999999999999999999999999999999")
	_, err = Decrypt(wrongKey, ciphertext)
	if err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	key := testKey()
	_, err := Decrypt(key, []byte{1, 2, 3})
	if err == nil {
		t.Error("Decrypt with too-short ciphertext should fail")
	}
}

// ── HMAC ───────────────────────────────────────────────────────────────────

func TestHMAC_Deterministic(t *testing.T) {
	key := testKey()
	data := []byte("test data")

	h1 := HMAC(key, data)
	h2 := HMAC(key, data)

	if h1 != h2 {
		t.Error("HMAC should be deterministic")
	}
}

func TestHMAC_DifferentData(t *testing.T) {
	key := testKey()

	h1 := HMAC(key, []byte("data1"))
	h2 := HMAC(key, []byte("data2"))

	if h1 == h2 {
		t.Error("HMAC of different data should differ")
	}
}

func TestHMAC_DifferentKeys(t *testing.T) {
	data := []byte("same data")

	h1 := HMAC([]byte("key1key1key1key1key1key1key1key1"), data)
	h2 := HMAC([]byte("key2key2key2key2key2key2key2key2"), data)

	if h1 == h2 {
		t.Error("HMAC with different keys should differ")
	}
}

func TestHMAC_Length(t *testing.T) {
	h := HMAC(testKey(), []byte("x"))
	// SHA-256 = 32 bytes = 64 hex chars
	if len(h) != 64 {
		t.Errorf("HMAC hex length = %d, want 64", len(h))
	}
}

// ── ParseKey ───────────────────────────────────────────────────────────────

func TestParseKey_Valid(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	b64 := base64.StdEncoding.EncodeToString(raw)

	key, err := ParseKey(b64)
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	if !bytes.Equal(key, raw) {
		t.Error("ParseKey returned wrong bytes")
	}
}

func TestParseKey_WrongLength(t *testing.T) {
	raw := make([]byte, 16) // too short
	b64 := base64.StdEncoding.EncodeToString(raw)

	_, err := ParseKey(b64)
	if err == nil {
		t.Error("ParseKey should reject non-32-byte keys")
	}
}

func TestParseKey_InvalidBase64(t *testing.T) {
	_, err := ParseKey("not!valid!base64!!!")
	if err == nil {
		t.Error("ParseKey should reject invalid base64")
	}
}
