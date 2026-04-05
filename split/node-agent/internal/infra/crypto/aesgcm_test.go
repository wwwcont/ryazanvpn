package crypto

import (
	"encoding/base64"
	"testing"
)

func TestAESGCMEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	svc, err := NewAESGCMServiceFromBase64(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	pt := []byte("hello")
	ct, err := svc.Encrypt(pt)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	out, err := svc.Decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(out) != string(pt) {
		t.Fatalf("want %s, got %s", pt, out)
	}
}
