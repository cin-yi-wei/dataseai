package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func randKey() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c, err := New(randKey())
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("hello prod-db password!")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, dec) {
		t.Fatalf("got %q, want %q", dec, plain)
	}
}

func TestEncrypt_DifferentCiphertextForSamePlaintext(t *testing.T) {
	c, _ := New(randKey())
	a, _ := c.Encrypt([]byte("same"))
	b, _ := c.Encrypt([]byte("same"))
	if bytes.Equal(a, b) {
		t.Fatal("nonce reuse — ciphertext should differ")
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	c1, _ := New(randKey())
	enc, _ := c1.Encrypt([]byte("secret"))
	c2, _ := New(randKey())
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatal("expected decrypt to fail with wrong key")
	}
}

func TestNew_RejectsBadKeyLength(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Fatal("16-byte key should be rejected")
	}
}
