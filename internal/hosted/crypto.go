package hosted

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

// DeriveKey derives a 32-byte AES key from CREDENTIAL_ENCRYPTION_KEY.
func DeriveKey(raw string) ([]byte, error) {
	raw = trimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY is required")
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

func EncryptionKeyFromEnv() ([]byte, error) {
	return DeriveKey(os.Getenv("CREDENTIAL_ENCRYPTION_KEY"))
}

func Encrypt(key, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func Decrypt(key []byte, ciphertext string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, payload := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	return gcm.Open(nil, nonce, payload, nil)
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
