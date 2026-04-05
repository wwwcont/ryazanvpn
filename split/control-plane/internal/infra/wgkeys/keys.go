package wgkeys

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/curve25519"
)

const keySize = 32

func GenerateKeyPair() (publicKey string, privateKey string, err error) {
	privateRaw := make([]byte, keySize)
	if _, err := rand.Read(privateRaw); err != nil {
		return "", "", err
	}
	clampPrivateKey(privateRaw)

	privateKey = base64.StdEncoding.EncodeToString(privateRaw)
	publicKey, err = DerivePublicKey(privateKey)
	if err != nil {
		return "", "", err
	}
	return publicKey, privateKey, nil
}

func DerivePublicKey(privateKeyBase64 string) (string, error) {
	privateRaw, err := decodePrivateKey(privateKeyBase64)
	if err != nil {
		return "", err
	}

	var private, public [keySize]byte
	copy(private[:], privateRaw)
	clampPrivateKey(private[:])
	curve25519.ScalarBaseMult(&public, &private)
	return base64.StdEncoding.EncodeToString(public[:]), nil
}

func PublicMatchesPrivate(privateKeyBase64 string, publicKeyBase64 string) (bool, error) {
	derived, err := DerivePublicKey(privateKeyBase64)
	if err != nil {
		return false, err
	}
	return derived == strings.TrimSpace(publicKeyBase64), nil
}

func ValidateKeyPair(privateKeyBase64 string, publicKeyBase64 string) error {
	ok, err := PublicMatchesPrivate(privateKeyBase64, publicKeyBase64)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("keypair mismatch: provided public key does not match private key")
	}
	return nil
}

func decodePrivateKey(privateKeyBase64 string) ([]byte, error) {
	privateRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateKeyBase64))
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	if len(privateRaw) != keySize {
		return nil, fmt.Errorf("invalid private key length: got %d", len(privateRaw))
	}
	return privateRaw, nil
}

func clampPrivateKey(privateRaw []byte) {
	if len(privateRaw) != keySize {
		return
	}
	privateRaw[0] &= 248
	privateRaw[31] &= 127
	privateRaw[31] |= 64
}
