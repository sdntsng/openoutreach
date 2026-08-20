package hosted

import (
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := DeriveKey("test-secret-key")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := Encrypt(key, []byte("refresh-token-value"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "refresh-token-value" {
		t.Fatalf("got %q", pt)
	}
}

func TestOpaqueToken(t *testing.T) {
	a, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) < 10 {
		t.Fatalf("tokens not opaque enough: %q %q", a, b)
	}
}

func TestSignTokenHMAC(t *testing.T) {
	sig := SignToken("secret", "tok-abc")
	if len(sig) != 16 {
		t.Fatalf("expected 16 hex chars, got %q", sig)
	}
	if SignToken("secret", "tok-abc") != sig {
		t.Fatal("hmac not deterministic")
	}
	if SignToken("other", "tok-abc") == sig {
		t.Fatal("different secret should change signature")
	}
}
