package wgkeys

import "testing"

func TestDerivePublicKey_KnownPair(t *testing.T) {
	const privateKey = "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI="
	const expectedPublic = "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE="

	got, err := DerivePublicKey(privateKey)
	if err != nil {
		t.Fatalf("DerivePublicKey error: %v", err)
	}
	if got != expectedPublic {
		t.Fatalf("public key mismatch: got=%s want=%s", got, expectedPublic)
	}
}

func TestGenerateKeyPair_Consistent(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error: %v", err)
	}

	ok, err := PublicMatchesPrivate(priv, pub)
	if err != nil {
		t.Fatalf("PublicMatchesPrivate error: %v", err)
	}
	if !ok {
		t.Fatalf("generated keypair is inconsistent")
	}
}

func TestValidateKeyPair(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error: %v", err)
	}
	if err := ValidateKeyPair(priv, pub); err != nil {
		t.Fatalf("ValidateKeyPair unexpected error: %v", err)
	}
	if err := ValidateKeyPair(priv, "KS3O5dK5fty5waMzWBFE92ovd3xpOEOEY6P2j84a+Cg="); err == nil {
		t.Fatal("expected mismatched keypair error")
	}
}
